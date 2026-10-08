package items

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrSaveOutcomeUnknown marks an insert that may or may not have been stored,
// for example because the connection failed before the database confirmed it.
var ErrSaveOutcomeUnknown = errors.New("item save outcome unknown")

// ErrExclusiveInsertsUnsupported is returned when the repository cannot hold
// off other inserts for an owner, so an import batch cannot check duplicates
// safely.
var ErrExclusiveInsertsUnsupported = errors.New("repository does not support exclusive item inserts")

// ExclusiveInserter is implemented by repositories that can make every other
// item insert for an owner wait, in every process sharing the store, while a
// batch reads the owner's items and inserts new ones.
type ExclusiveInserter interface {
	BeginExclusiveInserts(ctx context.Context, ownerID uuid.UUID) (InsertSession, error)
}

// InsertSession holds an owner's exclusive insert right until Close.
type InsertSession interface {
	// List returns the owner's items. No other insert for the owner can
	// happen until Close, so the result stays complete for the session.
	List(ctx context.Context) ([]Item, error)
	// Insert stores item, which must belong to the session's owner, as one
	// atomic write. An error that does not wrap ErrSaveOutcomeUnknown means
	// nothing was stored.
	Insert(ctx context.Context, item Item) (Item, error)
	// Close releases the owner's insert right. It is safe to call twice.
	Close()
}

// ImportBatch saves validated items for one owner while every other item
// insert for that owner waits, so duplicate checks against Existing stay
// accurate until Close.
type ImportBatch struct {
	session InsertSession
	ownerID uuid.UUID
}

// BeginImportBatch waits for and takes the owner's exclusive insert right. It
// fails with ErrExclusiveInsertsUnsupported rather than continuing without it.
func (s *Service) BeginImportBatch(ctx context.Context, ownerID uuid.UUID) (*ImportBatch, error) {
	if ownerID == uuid.Nil {
		return nil, validationErr("ownerID is required")
	}
	inserter, ok := s.repo.(ExclusiveInserter)
	if !ok {
		return nil, ErrExclusiveInsertsUnsupported
	}
	session, err := inserter.BeginExclusiveInserts(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return &ImportBatch{session: session, ownerID: ownerID}, nil
}

// Existing returns the owner's items as of now.
func (b *ImportBatch) Existing(ctx context.Context) ([]Item, error) {
	return b.session.List(ctx)
}

// Create validates input as Service.Create does and stores it for the batch
// owner. See InsertSession.Insert for what an error means.
func (b *ImportBatch) Create(ctx context.Context, input CreateItemInput) (Item, error) {
	input.OwnerID = b.ownerID
	item, err := newItem(input)
	if err != nil {
		return Item{}, err
	}
	item.ID = uuid.New()
	stampNewItem(&item, input)
	return b.session.Insert(ctx, item)
}

// Close releases the owner's insert right.
func (b *ImportBatch) Close() {
	b.session.Close()
}

// stampNewItem sets a new item's timestamps: now, unless input supplies them.
func stampNewItem(item *Item, input CreateItemInput) {
	item.CreatedAt = time.Now().UTC()
	if input.CreatedAt != nil && !input.CreatedAt.IsZero() {
		item.CreatedAt = input.CreatedAt.UTC()
	}
	item.UpdatedAt = item.CreatedAt
	if input.UpdatedAt != nil && !input.UpdatedAt.IsZero() {
		item.UpdatedAt = input.UpdatedAt.UTC()
	}
}
