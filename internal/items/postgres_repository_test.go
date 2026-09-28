package items

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"anthology/internal/platform/testdb"
)

// createTestUser inserts a user row (items reference users) and removes it,
// along with its items, when the test finishes.
func createTestUser(t *testing.T, db *sqlx.DB) uuid.UUID {
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

func TestPostgresRepositoryTargetedLookups(t *testing.T) {
	db := testdb.Open(t)
	assertTargetedLookups(t, NewPostgresRepository(db), createTestUser(t, db), createTestUser(t, db))
}
