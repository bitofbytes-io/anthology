package items

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/platform/testdb"
)

// TestPostgresInsertSessionExclusionAcrossConnectionPools runs the insert
// session contract on two repositories with separate connection pools, as two
// API replicas would have.
func TestPostgresInsertSessionExclusionAcrossConnectionPools(t *testing.T) {
	first, second := testdb.Open(t), testdb.Open(t)
	owner, other := createTestUser(t, first), createTestUser(t, first)
	assertInsertSessionExclusion(t, NewPostgresRepository(first), NewPostgresRepository(second), owner, other)
}

func TestPostgresInsertSessionRejectedInsertStoresNothing(t *testing.T) {
	db := testdb.Open(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	orphan := uuid.New() // no users row, so the owner foreign key rejects the insert
	session, err := repo.BeginExclusiveInserts(ctx, orphan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	item := newTestItem(orphan, "Orphan")
	_, err = session.Insert(ctx, item)
	if err == nil || errors.Is(err, ErrSaveOutcomeUnknown) {
		t.Fatalf("rejected insert = %v; want a confirmed failure", err)
	}
	var stored int
	if err := db.Get(&stored, `SELECT count(*) FROM items WHERE id = $1`, item.ID); err != nil || stored != 0 {
		t.Fatalf("rejected insert stored %d rows (%v)", stored, err)
	}
}

func TestPostgresInsertSessionLostConnectionIsUnknownOutcome(t *testing.T) {
	db := testdb.Open(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	owner := createTestUser(t, db)
	session, err := repo.BeginExclusiveInserts(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}

	var pid int
	if err := session.(*postgresInsertSession).conn.GetContext(ctx, &pid, `SELECT pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var alive bool
		if err := db.Get(&alive, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = $1)`, pid); err != nil {
			t.Fatal(err)
		}
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backend was not terminated")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, err := session.Insert(ctx, newTestItem(owner, "Lost reply")); !errors.Is(err, ErrSaveOutcomeUnknown) {
		t.Fatalf("insert on a dead connection = %v; want ErrSaveOutcomeUnknown", err)
	}
	session.Close()

	// The lock died with the session, so the owner is not left blocked.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	next, err := repo.BeginExclusiveInserts(waitCtx, owner)
	if err != nil {
		t.Fatalf("owner still blocked after the session's connection died: %v", err)
	}
	next.Close()
}

func TestPostgresCreateReturnsStoredRow(t *testing.T) {
	db := testdb.Open(t)
	repo := NewPostgresRepository(db)
	owner := createTestUser(t, db)
	item := newTestItem(owner, "Priced")
	price := 19.999
	item.RetailPriceUsd = &price

	stored, err := repo.Create(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != item.ID || stored.RetailPriceUsd == nil || *stored.RetailPriceUsd != 20 {
		t.Fatalf("Create returned %+v; want the stored row (price rounded to 20.00)", stored)
	}
}
