package items

import (
	"context"
	"errors"
	"slices"
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

// TestPostgresOwnerLockWaitersLeaveTheirPoolFree gives the waiters a pool of
// one connection. While another replica holds an owner's insert lock, a
// waiting batch and a waiting plain create for that owner must not occupy that
// connection, so another owner's inserts through the same pool still go
// through. Cancelled waits store nothing, the waiters still serialize behind
// the holder, and no connection or lock is left behind.
func TestPostgresOwnerLockWaitersLeaveTheirPoolFree(t *testing.T) {
	holderDB, smallDB := testdb.Open(t), testdb.Open(t)
	smallDB.SetMaxOpenConns(1)
	holder, small := NewPostgresRepository(holderDB), NewPostgresRepository(smallDB)
	busy, other := createTestUser(t, holderDB), createTestUser(t, holderDB)
	ctx := context.Background()

	held, err := holder.BeginExclusiveInserts(ctx, busy)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	batchSaw := make(chan []string, 1)
	go func() {
		session, err := small.BeginExclusiveInserts(ctx, busy)
		if err != nil {
			t.Errorf("waiting batch: %v", err)
			batchSaw <- nil
			return
		}
		defer session.Close()
		list, err := session.List(ctx)
		if err == nil {
			_, err = session.Insert(ctx, newTestItem(busy, "Waiting batch add"))
		}
		if err != nil {
			t.Errorf("waiting batch: %v", err)
		}
		titles := make([]string, 0, len(list))
		for _, item := range list {
			titles = append(titles, item.Title)
		}
		batchSaw <- titles
	}()
	createDone := make(chan error, 1)
	go func() {
		_, err := small.Create(ctx, newTestItem(busy, "Waiting manual add"))
		createDone <- err
	}()
	time.Sleep(100 * time.Millisecond) // let both waiters start polling

	otherCtx, cancelOther := context.WithTimeout(ctx, 3*time.Second)
	defer cancelOther()
	if _, err := small.Create(otherCtx, newTestItem(other, "Other owner add")); err != nil {
		t.Fatalf("another owner's create could not use the pool while same-owner requests waited: %v", err)
	}
	otherSession, err := small.BeginExclusiveInserts(otherCtx, other)
	if err != nil {
		t.Fatalf("another owner's batch could not use the pool while same-owner requests waited: %v", err)
	}
	if _, err := otherSession.Insert(otherCtx, newTestItem(other, "Other owner batch add")); err != nil {
		t.Fatal(err)
	}
	otherSession.Close()

	select {
	case <-batchSaw:
		t.Fatal("the waiting batch did not wait for the held lock")
	case <-createDone:
		t.Fatal("the waiting create did not wait for the held lock")
	case <-time.After(200 * time.Millisecond):
	}

	for name, wait := range map[string]func(context.Context) error{
		"create": func(ctx context.Context) error {
			_, err := small.Create(ctx, newTestItem(busy, "Cancelled manual add"))
			return err
		},
		"batch": func(ctx context.Context) error {
			session, err := small.BeginExclusiveInserts(ctx, busy)
			if err == nil {
				session.Close()
			}
			return err
		},
	} {
		waitCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		err := wait(waitCtx)
		cancel()
		if err == nil || waitCtx.Err() == nil {
			t.Fatalf("cancelled %s wait = %v; want it to give up when its context ended", name, err)
		}
	}

	if _, err := held.Insert(ctx, newTestItem(busy, "Held insert")); err != nil {
		t.Fatal(err)
	}
	held.Close()

	select {
	case titles := <-batchSaw:
		if !slices.Contains(titles, "Held insert") {
			t.Fatalf("waiting batch read %v before the holder finished", titles)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waiting batch never got the lock")
	}
	select {
	case err := <-createDone:
		if err != nil {
			t.Fatalf("waiting create: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waiting create never got the lock")
	}

	var titles []string
	if err := holderDB.Select(&titles, `SELECT title FROM items WHERE owner_id = $1 ORDER BY title`, busy); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Held insert", "Waiting batch add", "Waiting manual add"}; !slices.Equal(titles, want) {
		t.Fatalf("busy owner items = %v; want %v (cancelled waits must store nothing)", titles, want)
	}
	if inUse := smallDB.Stats().InUse; inUse != 0 {
		t.Fatalf("%d pooled connections still in use after every wait finished", inUse)
	}
	var locks int
	if err := holderDB.Get(&locks, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND classid = $1::bigint::oid AND objid = $2::bigint::oid AND objsubid = 2`,
		int64(uint32(itemInsertLockClass)), int64(uint32(ownerLockKey(busy)))); err != nil {
		t.Fatal(err)
	}
	if locks != 0 {
		t.Fatalf("%d owner insert locks still held after every session closed", locks)
	}
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
