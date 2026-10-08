package items

import (
	"context"
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// InMemoryRepository stores items in an in-process map, ideal for local development or tests.
type InMemoryRepository struct {
	mu    sync.RWMutex
	data  map[uuid.UUID]Item
	order []uuid.UUID

	// insertLocks holds one single-slot channel per owner; every insert for
	// the owner holds it, and an insert session holds it until Close.
	insertLocksMu sync.Mutex
	insertLocks   map[uuid.UUID]chan struct{}
}

// NewInMemoryRepository constructs a repository seeded with optional initial items.
func NewInMemoryRepository(initial []Item) *InMemoryRepository {
	data := make(map[uuid.UUID]Item)
	order := make([]uuid.UUID, 0, len(initial))
	for _, item := range initial {
		data[item.ID] = item
		order = append(order, item.ID)
	}
	return &InMemoryRepository{data: data, order: order, insertLocks: map[uuid.UUID]chan struct{}{}}
}

// lockInserts waits until no other insert or insert session for the owner is
// running, and returns the function that releases the owner's insert lock.
func (r *InMemoryRepository) lockInserts(ctx context.Context, ownerID uuid.UUID) (func(), error) {
	r.insertLocksMu.Lock()
	lock, ok := r.insertLocks[ownerID]
	if !ok {
		lock = make(chan struct{}, 1)
		r.insertLocks[ownerID] = lock
	}
	r.insertLocksMu.Unlock()

	release := func() { <-lock }
	select {
	case lock <- struct{}{}:
		return release, nil
	default:
	}
	select {
	case lock <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BeginExclusiveInserts makes every other insert for the owner wait until the
// returned session is closed.
func (r *InMemoryRepository) BeginExclusiveInserts(ctx context.Context, ownerID uuid.UUID) (InsertSession, error) {
	unlock, err := r.lockInserts(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return &memoryInsertSession{repo: r, ownerID: ownerID, unlock: unlock}, nil
}

type memoryInsertSession struct {
	repo    *InMemoryRepository
	ownerID uuid.UUID
	once    sync.Once
	unlock  func()
}

func (s *memoryInsertSession) List(ctx context.Context) ([]Item, error) {
	return s.repo.List(ctx, ListOptions{OwnerID: s.ownerID})
}

func (s *memoryInsertSession) Insert(_ context.Context, item Item) (Item, error) {
	if item.OwnerID != s.ownerID {
		return Item{}, validationErr("item belongs to a different owner than the insert session")
	}
	return s.repo.store(item), nil
}

func (s *memoryInsertSession) Close() {
	s.once.Do(s.unlock)
}

func (r *InMemoryRepository) store(item Item) Item {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[item.ID] = item
	r.order = append(r.order, item.ID)
	return item
}

// ownedItems yields the owner's items in insertion order. Callers must hold r.mu.
func (r *InMemoryRepository) ownedItems(ownerID uuid.UUID) iter.Seq[Item] {
	return func(yield func(Item) bool) {
		for _, id := range r.order {
			item, ok := r.data[id]
			if !ok || item.OwnerID != ownerID {
				continue
			}
			if !yield(item) {
				return
			}
		}
	}
}

// Create stores a new item, waiting while an insert session holds the owner's
// insert lock.
func (r *InMemoryRepository) Create(ctx context.Context, item Item) (Item, error) {
	unlock, err := r.lockInserts(ctx, item.OwnerID)
	if err != nil {
		return Item{}, err
	}
	defer unlock()
	return r.store(item), nil
}

// Get returns an item by ID and owner.
func (r *InMemoryRepository) Get(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.data[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if item.OwnerID != ownerID {
		return Item{}, ErrNotFound
	}
	return item, nil
}

// List returns stored items matching the supplied options.
func (r *InMemoryRepository) List(_ context.Context, opts ListOptions) ([]Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]Item, 0, len(r.order))
	queryFilter := ""
	if opts.Query != nil {
		queryFilter = strings.ToLower(strings.TrimSpace(*opts.Query))
	}

	for item := range r.ownedItems(opts.OwnerID) {
		if opts.ItemType != nil && item.ItemType != *opts.ItemType {
			continue
		}

		// When filtering by status with no type filter (All), only apply to books (BE-2)
		if opts.ReadingStatus != nil {
			if opts.ItemType == nil {
				// All items with status filter:
				// - "none" status: show all items (books with none + all non-books)
				// - Other statuses: show only books with that status
				if *opts.ReadingStatus == BookStatusNone {
					// For "none" status, show books with none status + all non-books
					if item.ItemType == ItemTypeBook && item.ReadingStatus != BookStatusNone {
						continue
					}
				} else {
					// For other statuses, only show books with that status
					if item.ItemType != ItemTypeBook {
						continue
					}
					if item.ReadingStatus != *opts.ReadingStatus {
						continue
					}
				}
			} else if item.ReadingStatus != *opts.ReadingStatus {
				// Specific type selected: apply status filter normally
				continue
			}
		}

		if opts.Initial != nil {
			initial := strings.ToUpper(strings.TrimSpace(*opts.Initial))
			titleInitial := ""
			trimmed := strings.TrimSpace(item.Title)
			if len(trimmed) > 0 {
				titleInitial = strings.ToUpper(string(trimmed[0]))
			}

			if initial == "#" {
				if titleInitial >= "A" && titleInitial <= "Z" {
					continue
				}
			} else {
				if titleInitial != initial {
					continue
				}
			}
		}

		if queryFilter != "" {
			title := strings.ToLower(strings.TrimSpace(item.Title))
			if !strings.Contains(title, queryFilter) {
				continue
			}
		}

		if opts.ShelfStatus != nil {
			switch *opts.ShelfStatus {
			case ShelfStatusOn:
				if item.ShelfPlacement == nil {
					continue
				}
			case ShelfStatusOff:
				if item.ShelfPlacement != nil {
					continue
				}
			}
		}

		items = append(items, item)
	}

	if opts.Limit != nil && *opts.Limit > 0 && len(items) > *opts.Limit {
		sorted := append([]Item(nil), items...)
		slices.SortFunc(sorted, compareItemsByCreatedDesc)
		items = sorted[:*opts.Limit]
	}

	return items, nil
}

// ListByIDs returns the owner's items with the given IDs.
func (r *InMemoryRepository) ListByIDs(_ context.Context, ids []uuid.UUID, ownerID uuid.UUID) ([]Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]Item, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		item, ok := r.data[id]
		if !ok || item.OwnerID != ownerID {
			continue
		}
		items = append(items, item)
	}
	slices.SortFunc(items, compareItemsByCreatedDesc)
	return items, nil
}

// FindByISBN returns the owner's most recently created book matching isbn, in
// either its ISBN-10 or ISBN-13 form, against either ISBN field (see
// isbnLookupKeys and isbnMatchKey).
func (r *InMemoryRepository) FindByISBN(_ context.Context, isbn string, ownerID uuid.UUID) (Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := isbnLookupKeys(isbn)
	if len(keys) == 0 {
		return Item{}, ErrNotFound
	}

	var (
		match Item
		found bool
	)
	for item := range r.ownedItems(ownerID) {
		if item.ItemType != ItemTypeBook {
			continue
		}
		if !slices.Contains(keys, isbnMatchKey(item.ISBN13)) && !slices.Contains(keys, isbnMatchKey(item.ISBN10)) {
			continue
		}
		if !found || compareItemsByCreatedDesc(item, match) < 0 {
			match = item
			found = true
		}
	}
	if !found {
		return Item{}, ErrNotFound
	}
	return match, nil
}

// Update replaces an existing item.
func (r *InMemoryRepository) Update(_ context.Context, item Item) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.data[item.ID]
	if !ok {
		return Item{}, ErrNotFound
	}
	if existing.OwnerID != item.OwnerID {
		return Item{}, ErrNotFound
	}
	r.data[item.ID] = item
	return item, nil
}

// Delete removes an item by ID and owner.
func (r *InMemoryRepository) Delete(_ context.Context, id uuid.UUID, ownerID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.data[id]
	if !ok {
		return ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if item.OwnerID != ownerID {
		return ErrNotFound
	}
	delete(r.data, id)
	for i, existing := range r.order {
		if existing == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return nil
}

// UpdateShelfPlacement updates the cached placement for an item.
func (r *InMemoryRepository) UpdateShelfPlacement(_ context.Context, itemID uuid.UUID, placement *ShelfPlacement) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.data[itemID]
	if !ok {
		return ErrNotFound
	}

	if placement == nil {
		item.ShelfPlacement = nil
	} else {
		copy := *placement
		item.ShelfPlacement = &copy
	}

	r.data[itemID] = item
	return nil
}

// Histogram returns a count of items grouped by first letter of title.
func (r *InMemoryRepository) Histogram(_ context.Context, opts HistogramOptions) (LetterHistogram, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	histogram := make(LetterHistogram)

	for item := range r.ownedItems(opts.OwnerID) {
		if opts.ItemType != nil && item.ItemType != *opts.ItemType {
			continue
		}

		// When filtering by status with no type filter (All), only apply to books (BE-2)
		if opts.ReadingStatus != nil {
			if opts.ItemType == nil {
				if *opts.ReadingStatus == BookStatusNone {
					if item.ItemType == ItemTypeBook && item.ReadingStatus != BookStatusNone {
						continue
					}
				} else {
					if item.ItemType != ItemTypeBook {
						continue
					}
					if item.ReadingStatus != *opts.ReadingStatus {
						continue
					}
				}
			} else if item.ReadingStatus != *opts.ReadingStatus {
				continue
			}
		}

		letter := extractFirstLetter(item.Title)
		histogram[letter]++
	}

	return histogram, nil
}

// extractFirstLetter returns the uppercase first letter of a title, or "#" for non-alphabetic.
func extractFirstLetter(title string) string {
	trimmed := strings.TrimSpace(title)
	if len(trimmed) == 0 {
		return "#"
	}

	first := strings.ToUpper(string(trimmed[0]))
	if first >= "A" && first <= "Z" {
		return first
	}
	return "#"
}

// FindDuplicates searches for items matching the given title or identifiers.
// Title matching is case-insensitive. Identifier matching normalizes by stripping non-digits.
// Returns up to 5 matches.
func (r *InMemoryRepository) FindDuplicates(_ context.Context, input DuplicateCheckInput, ownerID uuid.UUID) ([]DuplicateMatch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalizedTitle := NormalizeTitle(input.Title)
	normalizedISBN13 := NormalizeIdentifier(input.ISBN13)
	normalizedISBN10 := NormalizeIdentifier(input.ISBN10)

	// Track seen IDs to avoid duplicates in results
	seen := make(map[uuid.UUID]bool)
	var matches []DuplicateMatch

	const maxMatches = 5

	for item := range r.ownedItems(ownerID) {
		if len(matches) >= maxMatches {
			break
		}

		if seen[item.ID] {
			continue
		}

		matched := false

		// Check title match (case-insensitive, trimmed)
		if normalizedTitle != "" {
			itemTitle := NormalizeTitle(item.Title)
			if itemTitle == normalizedTitle {
				matched = true
			}
		}

		// Check ISBN13 match
		if !matched && normalizedISBN13 != "" {
			itemISBN13 := NormalizeIdentifier(item.ISBN13)
			if itemISBN13 != "" && itemISBN13 == normalizedISBN13 {
				matched = true
			}
		}

		// Check ISBN10 match
		if !matched && normalizedISBN10 != "" {
			itemISBN10 := NormalizeIdentifier(item.ISBN10)
			if itemISBN10 != "" && itemISBN10 == normalizedISBN10 {
				matched = true
			}
		}

		if matched {
			seen[item.ID] = true
			matches = append(matches, itemToDuplicateMatch(item))
		}
	}

	return matches, nil
}

// itemToDuplicateMatch converts an Item to a DuplicateMatch.
func itemToDuplicateMatch(item Item) DuplicateMatch {
	match := DuplicateMatch{
		ID:        item.ID,
		Title:     item.Title,
		CoverURL:  item.CoverImage,
		UpdatedAt: item.UpdatedAt,
	}

	// Set primary identifier (prefer ISBN13, then ISBN10)
	if item.ISBN13 != "" {
		match.PrimaryIdentifier = item.ISBN13
		match.IdentifierType = "ISBN-13"
	} else if item.ISBN10 != "" {
		match.PrimaryIdentifier = item.ISBN10
		match.IdentifierType = "ISBN-10"
	}

	// Set location from shelf placement if available
	if item.ShelfPlacement != nil {
		match.Location = item.ShelfPlacement.ShelfName
	}

	return match
}

// ListSeries returns all unique series with their items grouped.
func (r *InMemoryRepository) ListSeries(_ context.Context, opts SeriesRepoListOptions, ownerID uuid.UUID) ([]SeriesSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Group items by series name
	seriesMap := make(map[string][]Item)
	seriesOrder := []string{}

	for item := range r.ownedItems(ownerID) {
		// Only include books with series
		if item.ItemType != ItemTypeBook || item.SeriesName == "" {
			continue
		}

		if _, exists := seriesMap[item.SeriesName]; !exists {
			seriesOrder = append(seriesOrder, item.SeriesName)
		}
		seriesMap[item.SeriesName] = append(seriesMap[item.SeriesName], item)
	}

	// Build summaries
	summaries := make([]SeriesSummary, 0, len(seriesOrder))
	for _, name := range seriesOrder {
		items := seriesMap[name]

		// Sort items by volume number
		slices.SortFunc(items, func(a, b Item) int {
			if a.VolumeNumber == nil && b.VolumeNumber == nil {
				return strings.Compare(a.Title, b.Title)
			}
			if a.VolumeNumber == nil {
				return 1
			}
			if b.VolumeNumber == nil {
				return -1
			}
			return *a.VolumeNumber - *b.VolumeNumber
		})

		summary := SeriesSummary{
			SeriesName: name,
			OwnedCount: len(items),
		}

		if opts.IncludeItems {
			summary.Items = items
		}

		// Find max total_volumes from items
		for _, item := range items {
			if item.TotalVolumes != nil {
				if summary.TotalVolumes == nil || *item.TotalVolumes > *summary.TotalVolumes {
					summary.TotalVolumes = item.TotalVolumes
				}
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetSeriesByName returns detailed info about a single series.
func (r *InMemoryRepository) GetSeriesByName(ctx context.Context, name string, ownerID uuid.UUID) (SeriesSummary, error) {
	summaries, err := r.ListSeries(ctx, SeriesRepoListOptions{IncludeItems: true}, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}
	for _, summary := range summaries {
		if summary.SeriesName == name {
			return summary, nil
		}
	}
	return SeriesSummary{}, ErrNotFound
}

// ListSeriesNamesByNameCI returns distinct series names that match case-insensitively.
func (r *InMemoryRepository) ListSeriesNamesByNameCI(_ context.Context, name string, ownerID uuid.UUID) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if name == "" {
		return nil, nil
	}

	seen := make(map[string]struct{})
	for item := range r.ownedItems(ownerID) {
		if item.ItemType != ItemTypeBook || item.SeriesName == "" {
			continue
		}
		if strings.EqualFold(item.SeriesName, name) {
			seen[item.SeriesName] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(seen))
	for seriesName := range seen {
		names = append(names, seriesName)
	}
	slices.Sort(names)
	return names, nil
}

// UpdateSeriesName updates series_name on all items matching oldName for the given owner.
func (r *InMemoryRepository) UpdateSeriesName(_ context.Context, oldName, newName string, ownerID uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	for item := range r.ownedItems(ownerID) {
		if item.ItemType == ItemTypeBook && item.SeriesName == oldName {
			item.SeriesName = newName
			r.data[item.ID] = item
			count++
		}
	}
	return count, nil
}

// ClearSeriesName clears series_name, volume_number, and total_volumes on all items matching seriesName for the given owner.
func (r *InMemoryRepository) ClearSeriesName(_ context.Context, seriesName string, ownerID uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	for item := range r.ownedItems(ownerID) {
		if item.ItemType == ItemTypeBook && item.SeriesName == seriesName {
			item.SeriesName = ""
			item.VolumeNumber = nil
			item.TotalVolumes = nil
			r.data[item.ID] = item
			count++
		}
	}
	return count, nil
}
