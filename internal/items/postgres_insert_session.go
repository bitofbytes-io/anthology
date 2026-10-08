package items

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
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
	tryLockOwnerInsertsSQL      = `SELECT pg_try_advisory_xact_lock($1, $2)`
	tryLockOwnerInsertsUntilSQL = `SELECT pg_try_advisory_lock($1, $2)`
	unlockOwnerInsertsSQL       = `SELECT pg_advisory_unlock($1, $2)`
	insertSessionCloseTimeout   = 5 * time.Second
)

// Bounds of the randomized back-off between attempts to take a busy owner's
// insert lock.
const (
	ownerLockRetryMin = 10 * time.Millisecond
	ownerLockRetryMax = 250 * time.Millisecond
)

// waitForOwnerLock repeats attempt until it reports the owner's insert lock
// acquired or fails, backing off between attempts until ctx ends. An attempt
// that finds the lock busy must already have returned its connection to the
// pool, so a waiting request does not keep a pooled connection while it
// waits.
func waitForOwnerLock[T any](ctx context.Context, attempt func() (T, bool, error)) (T, error) {
	delay := ownerLockRetryMin
	for {
		result, acquired, err := attempt()
		if err != nil || acquired {
			return result, err
		}
		timer := time.NewTimer(delay/2 + rand.N(delay/2+1))
		select {
		case <-ctx.Done():
			timer.Stop()
			var zero T
			return zero, fmt.Errorf("wait for owner insert lock: %w", ctx.Err())
		case <-timer.C:
		}
		delay = min(delay*2, ownerLockRetryMax)
	}
}

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
// statement, so rows saved before a later failure stay saved. While another
// holder has the lock, it retries with back-off, returning its connection to
// the pool between attempts.
func (r *PostgresRepository) BeginExclusiveInserts(ctx context.Context, ownerID uuid.UUID) (InsertSession, error) {
	return waitForOwnerLock(ctx, func() (InsertSession, bool, error) {
		conn, err := r.db.Connx(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("open insert session: %w", err)
		}
		var acquired bool
		if err := conn.GetContext(ctx, &acquired, tryLockOwnerInsertsUntilSQL, itemInsertLockClass, ownerLockKey(ownerID)); err != nil {
			// The lock may have been granted just before the failure, so end
			// the database session rather than return a connection that could
			// hold it.
			discardConn(conn)
			return nil, false, fmt.Errorf("lock owner inserts: %w", err)
		}
		if !acquired {
			_ = conn.Close() // holds no lock, so it can go back to the pool
			return nil, false, nil
		}
		return &postgresInsertSession{repo: r, conn: conn, ownerID: ownerID}, true, nil
	})
}

// tryCreate makes one attempt to insert item under the owner's insert lock,
// in a transaction on its own connection. It reports acquired false, with
// the connection back in the pool, when another holder has the lock.
func (r *PostgresRepository) tryCreate(ctx context.Context, ownerID uuid.UUID, query string, args []any) (Item, bool, error) {
	conn, err := r.db.Connx(ctx)
	if err != nil {
		return Item{}, false, fmt.Errorf("begin item insert: %w", err)
	}
	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		discardConn(conn)
		return Item{}, false, fmt.Errorf("begin item insert: %w", err)
	}
	// finish ends the transaction and returns the connection to the pool, or
	// closes it if the rollback fails.
	finish := func() {
		if err := tx.Rollback(); err != nil {
			discardConn(conn)
			return
		}
		_ = conn.Close()
	}

	var acquired bool
	if err := tx.GetContext(ctx, &acquired, tryLockOwnerInsertsSQL, itemInsertLockClass, ownerLockKey(ownerID)); err != nil {
		// Ending the session releases a lock that may have been granted.
		_ = tx.Rollback()
		discardConn(conn)
		return Item{}, false, fmt.Errorf("lock owner inserts: %w", err)
	}
	if !acquired {
		finish()
		return Item{}, false, nil
	}

	var stored Item
	if err := tx.QueryRowxContext(ctx, query, args...).StructScan(&stored); err != nil {
		// Nothing is committed until Commit, so a failure here stores nothing.
		finish()
		return Item{}, true, fmt.Errorf("insert item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		discardConn(conn)
		return Item{}, true, saveOutcomeErr(ctx, "commit item insert", err)
	}
	_ = conn.Close()
	return stored, true, nil
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
