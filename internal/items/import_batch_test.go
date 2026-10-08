package items

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

// exclusiveRepo is a repository that supports insert sessions.
type exclusiveRepo interface {
	Repository
	ExclusiveInserter
}

func newTestItem(owner uuid.UUID, title string) Item {
	now := time.Now().UTC().Truncate(time.Second)
	return Item{ID: uuid.New(), OwnerID: owner, Title: title, ItemType: ItemTypeBook, ISBN13: "9780441172719", ReadingStatus: BookStatusNone, Format: FormatUnknown, CreatedAt: now, UpdatedAt: now}
}

// assertBlockedUntil checks that done stays open while the session is held
// and closes once release has run.
func assertBlockedUntil(t *testing.T, what string, done <-chan struct{}, release func()) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("%s did not wait for the open insert session", what)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s still waiting after the insert session closed", what)
	}
}

// assertInsertSessionExclusion checks the import batch contract on two
// repository instances that share one store (two API replicas): while a
// session for owner is open, another session or a plain Create for owner
// waits, the waiting session then sees the first session's insert, and
// another owner is not held up.
func assertInsertSessionExclusion(t *testing.T, first, second exclusiveRepo, owner, other uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	session, err := first.BeginExclusiveInserts(ctx, owner)
	if err != nil {
		t.Fatalf("begin first session: %v", err)
	}
	defer session.Close()
	if list, err := session.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("first session list = %d items, %v; want empty", len(list), err)
	}

	// Another owner is independent of the open session.
	otherDone := make(chan error, 1)
	go func() {
		otherSession, err := second.BeginExclusiveInserts(ctx, other)
		if err == nil {
			_, err = otherSession.Insert(ctx, newTestItem(other, "Other owner book"))
			otherSession.Close()
		}
		otherDone <- err
	}()
	select {
	case err := <-otherDone:
		if err != nil {
			t.Fatalf("other owner session: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("another owner's session waited for this owner's session")
	}

	var secondList []Item
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		waiting, err := second.BeginExclusiveInserts(ctx, owner)
		if err != nil {
			t.Errorf("begin second session: %v", err)
			return
		}
		defer waiting.Close()
		if secondList, err = waiting.List(ctx); err != nil {
			t.Errorf("second session list: %v", err)
		}
	}()
	assertBlockedUntil(t, "a second insert session", secondDone, func() {
		if _, err := session.Insert(ctx, newTestItem(owner, "Same reviewed book")); err != nil {
			t.Fatalf("first session insert: %v", err)
		}
		session.Close()
	})
	if len(secondList) != 1 || secondList[0].Title != "Same reviewed book" {
		t.Fatalf("waiting session saw %+v; want the first session's insert", secondList)
	}

	held, err := first.BeginExclusiveInserts(ctx, owner)
	if err != nil {
		t.Fatalf("begin held session: %v", err)
	}
	createDone := make(chan struct{})
	go func() {
		defer close(createDone)
		if _, err := second.Create(ctx, newTestItem(owner, "Manual add")); err != nil {
			t.Errorf("plain create: %v", err)
		}
	}()
	assertBlockedUntil(t, "a plain create", createDone, held.Close)

	list, err := first.List(ctx, ListOptions{OwnerID: owner})
	if err != nil || len(list) != 2 {
		t.Fatalf("owner items = %d, %v; want 2", len(list), err)
	}
}

func TestInMemoryInsertSessionExclusion(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	assertInsertSessionExclusion(t, repo, repo, uuid.New(), uuid.New())
}

func TestInMemoryInsertSessionWaitHonoursContext(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	owner := uuid.New()
	session, err := repo.BeginExclusiveInserts(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := repo.BeginExclusiveInserts(ctx, owner); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting session = %v; want deadline exceeded", err)
	}
	if _, err := repo.Create(ctx, newTestItem(owner, "Manual add")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting create = %v; want deadline exceeded", err)
	}
	if _, err := session.Insert(context.Background(), newTestItem(uuid.New(), "Wrong owner")); !errors.Is(err, ErrValidation) {
		t.Fatalf("insert for another owner = %v; want validation error", err)
	}
}

func TestBeginImportBatchRequiresExclusiveInserts(t *testing.T) {
	svc := NewService(struct{ Repository }{NewInMemoryRepository(nil)})
	if _, err := svc.BeginImportBatch(context.Background(), uuid.New()); !errors.Is(err, ErrExclusiveInsertsUnsupported) {
		t.Fatalf("BeginImportBatch = %v; want ErrExclusiveInsertsUnsupported", err)
	}
}

func TestImportBatchCreateValidatesAndForcesOwner(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	owner := uuid.New()
	batch, err := svc.BeginImportBatch(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Close()
	if _, err := batch.Create(context.Background(), CreateItemInput{ItemType: ItemTypeBook}); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid item = %v; want validation error", err)
	}
	item, err := batch.Create(context.Background(), CreateItemInput{OwnerID: uuid.New(), Title: " Spaced ", ItemType: ItemTypeBook})
	if err != nil {
		t.Fatal(err)
	}
	if item.OwnerID != owner || item.Title != "Spaced" || item.CreatedAt.IsZero() {
		t.Fatalf("unexpected item %+v", item)
	}
}

func TestNonFinitePriceFailsValidation(t *testing.T) {
	for _, price := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := NormalizeCreateInput(CreateItemInput{Title: "T", ItemType: ItemTypeBook, RetailPriceUsd: &price})
		if !errors.Is(err, ErrValidation) || err.Error() != "retailPriceUsd must be a finite number" {
			t.Fatalf("price %v: got %v", price, err)
		}
	}
}
