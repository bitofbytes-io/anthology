package items

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestServiceUpdateSeriesNameReturnsUnexpectedLookupError(t *testing.T) {
	repo := &seriesUpdateRepo{
		t: t,
		summaries: map[string]SeriesSummary{
			"Old Series": {SeriesName: "Old Series"},
		},
	}
	lookupErr := errors.New("lookup failed")
	repo.errByName = map[string]error{"New Series": lookupErr}

	svc := NewService(repo)

	_, err := svc.UpdateSeriesName(context.Background(), "Old Series", "New Series", testOwnerID)
	if err == nil || !errors.Is(err, lookupErr) {
		t.Fatalf("expected lookup error to propagate, got %v", err)
	}
	if repo.updateCalled {
		t.Fatalf("expected UpdateSeriesName not to be called after lookup error")
	}
}

func TestServiceUpdateSeriesNameRejectsCaseInsensitiveCollision(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateItemInput{
		OwnerID:    testOwnerID,
		Title:      "First Book",
		ItemType:   ItemTypeBook,
		SeriesName: "abc",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = svc.Create(ctx, CreateItemInput{
		OwnerID:    testOwnerID,
		Title:      "Second Book",
		ItemType:   ItemTypeBook,
		SeriesName: "ABC",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = svc.UpdateSeriesName(ctx, "abc", "Abc", testOwnerID)
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for case-insensitive collision, got %v", err)
	}
}

func TestServiceEnrichSeriesSummarySetsMissingCountWhenCompleteWithoutVolumeNumbers(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	summary := SeriesSummary{
		SeriesName:   "Saga",
		OwnedCount:   3,
		TotalVolumes: ptrInt(3),
		Items: []Item{
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
		},
	}

	enriched := svc.enrichSeriesSummary(summary)

	if enriched.MissingCount == nil || *enriched.MissingCount != 0 {
		t.Fatalf("expected missingCount to be 0, got %#v", enriched.MissingCount)
	}
	if enriched.Status != SeriesStatusComplete {
		t.Fatalf("expected status to be complete, got %q", enriched.Status)
	}
}

func TestServiceEnrichSeriesSummaryMarksIncompleteWhenTotalsExceedOwnedWithoutVolumeNumbers(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	summary := SeriesSummary{
		SeriesName:   "Saga",
		OwnedCount:   2,
		TotalVolumes: ptrInt(4),
		Items: []Item{
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
		},
	}

	enriched := svc.enrichSeriesSummary(summary)

	if enriched.MissingCount == nil || *enriched.MissingCount != 2 {
		t.Fatalf("expected missingCount to be 2, got %#v", enriched.MissingCount)
	}
	if enriched.Status != SeriesStatusIncomplete {
		t.Fatalf("expected status to be incomplete, got %q", enriched.Status)
	}
}

type seriesUpdateRepo struct {
	t            *testing.T
	updateCalled bool
	summaries    map[string]SeriesSummary
	errByName    map[string]error
}

func (r *seriesUpdateRepo) Create(context.Context, Item) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Create call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) Get(context.Context, uuid.UUID, uuid.UUID) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Get call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) List(context.Context, ListOptions) ([]Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected List call")
	return nil, nil
}

func (r *seriesUpdateRepo) ListByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected ListByIDs call")
	return nil, nil
}

func (r *seriesUpdateRepo) FindByISBN(context.Context, string, uuid.UUID) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected FindByISBN call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) Update(context.Context, Item) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Update call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	r.t.Helper()
	r.t.Fatalf("unexpected Delete call")
	return nil
}

func (r *seriesUpdateRepo) Histogram(context.Context, HistogramOptions) (LetterHistogram, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Histogram call")
	return nil, nil
}

func (r *seriesUpdateRepo) FindDuplicates(context.Context, DuplicateCheckInput, uuid.UUID) ([]DuplicateMatch, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected FindDuplicates call")
	return nil, nil
}

func (r *seriesUpdateRepo) ListSeries(context.Context, SeriesRepoListOptions, uuid.UUID) ([]SeriesSummary, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected ListSeries call")
	return nil, nil
}

func (r *seriesUpdateRepo) GetSeriesByName(_ context.Context, name string, _ uuid.UUID) (SeriesSummary, error) {
	if err, ok := r.errByName[name]; ok {
		return SeriesSummary{}, err
	}
	if summary, ok := r.summaries[name]; ok {
		return summary, nil
	}
	return SeriesSummary{}, ErrNotFound
}

func (r *seriesUpdateRepo) ListSeriesNamesByNameCI(_ context.Context, name string, _ uuid.UUID) ([]string, error) {
	if err, ok := r.errByName[name]; ok {
		return nil, err
	}
	names := []string{}
	for seriesName := range r.summaries {
		if strings.EqualFold(seriesName, name) {
			names = append(names, seriesName)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (r *seriesUpdateRepo) UpdateSeriesName(context.Context, string, string, uuid.UUID) (int64, error) {
	r.updateCalled = true
	return 1, nil
}

func (r *seriesUpdateRepo) ClearSeriesName(context.Context, string, uuid.UUID) (int64, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected ClearSeriesName call")
	return 0, nil
}
