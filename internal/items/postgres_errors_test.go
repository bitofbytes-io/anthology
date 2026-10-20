package items

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestPostgresLookupsClassifyWrappedDatabaseErrors(t *testing.T) {
	queryFailure := errors.New("query failed")
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"missing", sql.ErrNoRows, ErrNotFound},
		{"wrapped missing", fmt.Errorf("driver lookup: %w", sql.ErrNoRows), ErrNotFound},
		{"query failure", fmt.Errorf("driver lookup: %w", queryFailure), queryFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := sqlx.NewDb(sql.OpenDB(queryErrorDriver{err: tc.err}), "postgres")
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Errorf("close test database: %v", err)
				}
			})
			repo := NewPostgresRepository(db)
			owner := uuid.New()
			for name, lookup := range map[string]func() (Item, error){
				"Get": func() (Item, error) {
					return repo.Get(t.Context(), uuid.New(), owner)
				},
				"FindByISBN": func() (Item, error) {
					return repo.FindByISBN(t.Context(), "9780140328721", owner)
				},
			} {
				t.Run(name, func(t *testing.T) {
					_, err := lookup()
					if !errors.Is(err, tc.want) {
						t.Fatalf("lookup error = %v; want %v", err, tc.want)
					}
					if errors.Is(tc.want, queryFailure) && errors.Is(err, ErrNotFound) {
						t.Fatalf("database failure was misclassified as missing: %v", err)
					}
				})
			}
		})
	}
}

// This driver injects errors at the database/sql boundary, so both sqlx lookup
// paths are exercised without relying on a real driver to wrap ErrNoRows.
type queryErrorDriver struct{ err error }

func (d queryErrorDriver) Open(string) (driver.Conn, error) {
	return queryErrorConn(d), nil
}
func (d queryErrorDriver) Connect(context.Context) (driver.Conn, error) { return d.Open("") }
func (d queryErrorDriver) Driver() driver.Driver                        { return d }

type queryErrorConn struct{ err error }

func (c queryErrorConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, c.err
}
func (queryErrorConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}
func (queryErrorConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected Begin") }
func (queryErrorConn) Close() error              { return nil }

func TestOwnerLockKeyPreservesSignedBitPattern(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		want   int32
	}{
		{"00000000", 0},
		{"7fffffff", 2147483647},
		{"80000000", -2147483648},
		{"ffffffff", -1},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			owner := uuid.MustParse(tc.prefix + "-0000-4000-8000-000000000001")
			if got := ownerLockKey(owner); got != tc.want {
				t.Fatalf("owner lock key = %d; want %d for rolling-version compatibility", got, tc.want)
			}
		})
	}
}
