package importer

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"anthology/internal/items"
	"anthology/internal/platform/testdb"
)

func createImportTestUser(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()
	owner := uuid.New()
	if _, err := db.Exec(`INSERT INTO users(id,email,oauth_provider,oauth_provider_id) VALUES($1,$2,'test',$2)`, owner, owner.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM items WHERE owner_id=$1`, owner)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, owner)
	})
	return owner
}

// TestPostgresOverlappingCommitsAcrossPoolsSaveOnce commits the same reviewed
// row many times at once through two importers with separate connection
// pools, as two API replicas would: each owner ends up with exactly one item,
// one commit reports it added and the rest skipped. Owners stay independent.
func TestPostgresOverlappingCommitsAcrossPoolsSaveOnce(t *testing.T) {
	first, second := testdb.Open(t), testdb.Open(t)
	importers := []*CSVImporter{
		NewCSVImporter(items.NewService(items.NewPostgresRepository(first)), nil),
		NewCSVImporter(items.NewService(items.NewPostgresRepository(second)), nil),
	}
	owners := []uuid.UUID{createImportTestUser(t, first), createImportTestUser(t, first), createImportTestUser(t, first)}
	const commitsPerOwner = 6

	type outcome struct {
		owner  uuid.UUID
		result CommitResult
		err    error
	}
	outcomes := make(chan outcome, len(owners)*commitsPerOwner)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, owner := range owners {
		for n := range commitsPerOwner {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				result, err := importers[n%2].Commit(context.Background(), sameBookRequest(2), owner)
				outcomes <- outcome{owner, result, err}
			}()
		}
	}
	close(start)
	wg.Wait()
	close(outcomes)

	added, skipped := map[uuid.UUID]int{}, map[uuid.UUID]int{}
	for o := range outcomes {
		if o.err != nil {
			t.Fatalf("commit: %v", o.err)
		}
		added[o.owner] += o.result.Added
		skipped[o.owner] += o.result.Skipped
	}
	reader := items.NewService(items.NewPostgresRepository(first))
	for _, owner := range owners {
		saved, err := reader.List(context.Background(), items.ListOptions{OwnerID: owner})
		if err != nil {
			t.Fatal(err)
		}
		if len(saved) != 1 || added[owner] != 1 || skipped[owner] != commitsPerOwner-1 {
			t.Fatalf("owner %s: %d saved, %d added, %d skipped; want 1 saved, 1 added, %d skipped",
				owner, len(saved), added[owner], skipped[owner], commitsPerOwner-1)
		}
	}
}
