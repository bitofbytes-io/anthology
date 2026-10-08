package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

func sameBookRequest(rows ...int) CommitRequest {
	request := CommitRequest{}
	for _, row := range rows {
		request.Rows = append(request.Rows, ReviewedRow{Row: row, Item: ReviewedItem{CreateItemInput: items.CreateItemInput{
			Title: "Same reviewed book", ItemType: items.ItemTypeBook, ISBN13: "9780441172719",
		}}})
	}
	return request
}

// TestCommitOverlappingSameOwnerSavesOnce holds the first commit right after
// it reads the library, then starts a second commit of the same row: the
// second must not read the library until the first is done, so it skips the
// row instead of saving it again. Another owner's commit is not held up.
func TestCommitOverlappingSameOwnerSavesOnce(t *testing.T) {
	store := newLibraryStore(t)
	importer := NewCSVImporter(store, nil)
	var lists atomic.Int32
	firstListed, release := make(chan struct{}), make(chan struct{})
	store.repo.onList = func() {
		if lists.Add(1) == 1 {
			close(firstListed)
			<-release
		}
	}

	var results [2]CommitResult
	var wg sync.WaitGroup
	commit := func(n int) {
		defer wg.Done()
		result, err := importer.Commit(context.Background(), sameBookRequest(2), testOwnerID)
		if err != nil {
			t.Errorf("commit %d: %v", n, err)
		}
		results[n] = result
	}
	wg.Add(1)
	go commit(0)
	<-firstListed

	otherDone := make(chan CommitResult, 1)
	go func() {
		result, err := importer.Commit(context.Background(), sameBookRequest(2), otherOwnerID)
		if err != nil {
			t.Errorf("other owner commit: %v", err)
		}
		otherDone <- result
	}()
	select {
	case result := <-otherDone:
		if result.Added != 1 {
			t.Fatalf("another owner's commit must save its row, got %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("another owner's commit waited for this owner's import")
	}

	wg.Add(1)
	go commit(1)
	time.Sleep(100 * time.Millisecond)
	if got := lists.Load(); got != 2 {
		t.Fatalf("the overlapping commit read the library while the first held the owner's insert right (%d reads)", got)
	}
	close(release)
	wg.Wait()

	if results[0].Added != 1 || results[1].Skipped != 1 || results[1].Added != 0 {
		t.Fatalf("expected first added and second skipped, got %+v and %+v", results[0], results[1])
	}
	if matches := results[1].Rows[0].LibraryMatches; len(matches) == 0 || matches[0].ItemID != *results[0].Rows[0].ItemID {
		t.Fatalf("expected the skip to point at the first commit's item, got %+v", results[1].Rows[0])
	}
	if got := len(store.owned(t, testOwnerID)); got != 1 {
		t.Fatalf("same reviewed row saved %d times", got)
	}
}

// TestCommitUnknownSaveIsNotReportedFailedAndStops covers an insert that
// reached the store but whose confirmation was lost: the row is reported as
// unconfirmed, not failed, and nothing else is inserted after it.
func TestCommitUnknownSaveIsNotReportedFailedAndStops(t *testing.T) {
	store := newLibraryStore(t)
	store.repo.fault = func(ctx context.Context, call int32, insert func() (items.Item, error)) (items.Item, error) {
		if call == 1 {
			if _, err := insert(); err != nil {
				return items.Item{}, err
			}
			return items.Item{}, fmt.Errorf("insert item: %w: read reply: connection reset by peer", items.ErrSaveOutcomeUnknown)
		}
		return insert()
	}
	importer := NewCSVImporter(store, nil)
	request := sameBookRequest(2, 3)
	request.Rows = append(request.Rows, ReviewedRow{Row: 4, Item: ReviewedItem{CreateItemInput: items.CreateItemInput{Title: "Other book", ItemType: items.ItemTypeMovie}}})

	result, err := importer.Commit(context.Background(), request, testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	statuses := []OutcomeStatus{result.Rows[0].Status, result.Rows[1].Status, result.Rows[2].Status}
	if !slices.Equal(statuses, []OutcomeStatus{OutcomeInterrupted, OutcomeUnprocessed, OutcomeUnprocessed}) {
		t.Fatalf("unexpected statuses %v", statuses)
	}
	if result.Failed != 0 || result.Added != 0 || result.Interrupted != 1 || result.Unprocessed != 2 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if result.Rows[0].Message != msgCommitInterrupted || result.Rows[1].Message != msgCommitAfterUnknown {
		t.Fatalf("unexpected messages: %+v", result.Rows)
	}
	if got := store.creates.Load(); got != 1 {
		t.Fatalf("expected no insert after the unconfirmed one, got %d inserts", got)
	}
	if got := len(store.owned(t, testOwnerID)); got != 1 {
		t.Fatalf("expected the unconfirmed row stored once, found %d items", got)
	}

	// Retrying the same request skips what the lost save stored.
	store.repo.fault = nil
	retry, err := importer.Commit(context.Background(), request, testOwnerID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retry.Skipped != 2 || retry.Added != 1 || len(store.owned(t, testOwnerID)) != 2 {
		t.Fatalf("retry must skip the stored row and add the other, got %+v", retry)
	}
}

// TestPreviewRejectsNonFiniteCatalogPrice keeps a non-finite price from a
// catalog match out of the preview, so the response can always be encoded.
func TestPreviewRejectsNonFiniteCatalogPrice(t *testing.T) {
	nan := math.NaN()
	lookups := &editionCatalog{results: map[string][]catalog.Metadata{
		"9780000000100": {{Title: "Odd Price", ISBN13: "9780000000100", RetailPriceUsd: &nan}},
	}}
	preview, err := NewCSVImporter(newLibraryStore(t), lookups).Preview(context.Background(), strings.NewReader(reviewHeader+",,book,,,9780000000100,,,,,,\n"), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	row := preview.Rows[0]
	if row.Status != RowNeedsMatch || len(row.Candidates) != 0 || !strings.Contains(row.Problem, "retailPriceUsd must be a finite number") {
		t.Fatalf("expected the non-finite candidate to be rejected, got %+v", row)
	}
	if _, err := json.Marshal(preview); err != nil {
		t.Fatalf("preview must encode as JSON: %v", err)
	}
}

func TestCommitRefusesStoresWithoutExclusiveInserts(t *testing.T) {
	request := sameBookRequest(2)
	if _, err := NewCSVImporter(&stubStore{}, nil).Commit(context.Background(), request, testOwnerID); !errors.Is(err, ErrReviewedImportUnavailable) {
		t.Fatalf("store without import batches: got %v", err)
	}

	// Hide the in-memory repository's exclusive inserts behind the plain
	// Repository interface.
	repo := struct{ items.Repository }{items.NewInMemoryRepository(nil)}
	service := items.NewService(repo)
	if _, err := NewCSVImporter(service, nil).Commit(context.Background(), request, testOwnerID); !errors.Is(err, ErrReviewedImportUnavailable) {
		t.Fatalf("repository without exclusive inserts: got %v", err)
	}
	if list, _ := service.List(context.Background(), items.ListOptions{OwnerID: testOwnerID}); len(list) != 0 {
		t.Fatalf("refused commit saved %d items", len(list))
	}
}

func TestCommitWaitingForBusyOwnerReportsRowsUnprocessed(t *testing.T) {
	store := newLibraryStore(t)
	held, err := store.BeginImportBatch(context.Background(), testOwnerID)
	if err != nil {
		t.Fatalf("hold batch: %v", err)
	}
	defer held.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := NewCSVImporter(store, nil).Commit(ctx, sameBookRequest(2, 3), testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if result.Unprocessed != 2 || result.Rows[0].Message != msgCommitLockTimeout || store.creates.Load() != 0 {
		t.Fatalf("expected both rows unprocessed and nothing saved, got %+v", result)
	}
}
