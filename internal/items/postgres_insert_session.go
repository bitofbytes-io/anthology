package items

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const insertItemSQL = `INSERT INTO items (id, owner_id, title, creator, item_type, release_year, page_count, current_page, isbn_13, isbn_10, description, cover_image, format, genre, rating, retail_price_usd, google_volume_id, platform, age_group, player_count, reading_status, read_at, notes, series_name, volume_number, total_volumes, created_at, updated_at)
VALUES (:id, :owner_id, :title, :creator, :item_type, :release_year, :page_count, :current_page, :isbn_13, :isbn_10, :description, :cover_image, :format, :genre, :rating, :retail_price_usd, :google_volume_id, :platform, :age_group, :player_count, :reading_status, :read_at, :notes, :series_name, :volume_number, :total_volumes, :created_at, :updated_at)
RETURNING id, owner_id, title, creator, item_type, release_year, page_count, current_page, isbn_13, isbn_10, description, cover_image, format, genre, rating, retail_price_usd, google_volume_id, platform, age_group, player_count, reading_status, read_at, notes, series_name, volume_number, total_volumes, created_at, updated_at`

// itemInsertLockClass is the first key of the two-key advisory lock that
// serializes an owner's item inserts ("anth"). Two-key advisory locks never
// conflict with single-key ones such as the migration lock.
const itemInsertLockClass int32 = 0x616e7468

const (
	lockOwnerInsertsSQL       = `SELECT pg_advisory_xact_lock($1, $2)`
	lockOwnerInsertsUntilSQL  = `SELECT pg_advisory_lock($1, $2)`
	unlockOwnerInsertsSQL     = `SELECT pg_advisory_unlock($1, $2)`
	insertSessionCloseTimeout = 5 * time.Second
)

// ownerLockKey derives the second advisory lock key from the owner ID. Two
// owners may share a key; that only makes their inserts wait for each other.
func ownerLockKey(ownerID uuid.UUID) int32 {
	return int32(binary.BigEndian.Uint32(ownerID[:4]))
}

// saveOutcomeErr classifies a failed write. PostgreSQL reporting an ordinary
// error means the statement or transaction was rolled back, so nothing was
// stored. Anything else (a lost connection, a cancelled request, a fatal
// server error, or a failure reading the reply) may have followed a
// successful commit, so the outcome is unknown.
func saveOutcomeErr(ctx context.Context, action string, err error) error {
	var pqErr *pq.Error
	if ctx.Err() == nil && errors.As(err, &pqErr) && !pqErr.Fatal() {
		switch pqErr.SQLState()[:2] {
		case "08", "57", "58", "XX": // connection, operator intervention, system, internal
		default:
			return fmt.Errorf("%s: %w", action, err)
		}
	}
	return fmt.Errorf("%s: %w: %w", action, ErrSaveOutcomeUnknown, err)
}

// BeginExclusiveInserts pins a connection and takes the owner's insert lock
// at session level, so every other insert for the owner, from any process
// using this database, waits until Close. Each Insert is its own autocommitted
// statement, so rows saved before a later failure stay saved.
func (r *PostgresRepository) BeginExclusiveInserts(ctx context.Context, ownerID uuid.UUID) (InsertSession, error) {
	conn, err := r.db.Connx(ctx)
	if err != nil {
		return nil, fmt.Errorf("open insert session: %w", err)
	}
	if _, err := conn.ExecContext(ctx, lockOwnerInsertsUntilSQL, itemInsertLockClass, ownerLockKey(ownerID)); err != nil {
		// The lock may have been granted just before the failure, so end the
		// database session rather than return a connection that could hold it.
		discardConn(conn)
		return nil, fmt.Errorf("lock owner inserts: %w", err)
	}
	return &postgresInsertSession{repo: r, conn: conn, ownerID: ownerID}, nil
}

type postgresInsertSession struct {
	repo    *PostgresRepository
	conn    *sqlx.Conn
	ownerID uuid.UUID
	once    sync.Once
}

func (s *postgresInsertSession) List(ctx context.Context) ([]Item, error) {
	rows := []itemRow{}
	if err := s.conn.SelectContext(ctx, &rows, baseSelect+" WHERE i.owner_id = $1", s.ownerID); err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toItem())
	}
	return items, nil
}

func (s *postgresInsertSession) Insert(ctx context.Context, item Item) (Item, error) {
	if item.OwnerID != s.ownerID {
		return Item{}, validationErr("item belongs to a different owner than the insert session")
	}
	query, args, err := s.repo.db.BindNamed(insertItemSQL, item)
	if err != nil {
		return Item{}, fmt.Errorf("bind item insert: %w", err)
	}
	var stored Item
	if err := s.conn.QueryRowxContext(ctx, query, args...).StructScan(&stored); err != nil {
		return Item{}, saveOutcomeErr(ctx, "insert item", err)
	}
	return stored, nil
}

// Close releases the lock and returns the connection to the pool, or ends
// the database session (which releases its locks) if the unlock fails.
func (s *postgresInsertSession) Close() {
	s.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), insertSessionCloseTimeout)
		defer cancel()
		var released bool
		err := s.conn.GetContext(ctx, &released, unlockOwnerInsertsSQL, itemInsertLockClass, ownerLockKey(s.ownerID))
		if err != nil || !released {
			discardConn(s.conn)
			return
		}
		_ = s.conn.Close()
	})
}

// discardConn closes the underlying database connection instead of returning
// it to the pool, which ends the session and releases any advisory locks.
func discardConn(conn *sqlx.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}
