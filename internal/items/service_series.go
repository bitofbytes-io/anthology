package items

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// ListSeries returns all series with aggregated statistics and missing volume detection.
func (s *Service) ListSeries(ctx context.Context, opts SeriesListOptions, ownerID uuid.UUID) (SeriesListResponse, error) {
	// Always include items for missing volume calculation.
	repoOpts := SeriesRepoListOptions{
		IncludeItems: true,
	}

	summaries, err := s.repo.ListSeries(ctx, repoOpts, ownerID)
	if err != nil {
		return SeriesListResponse{}, err
	}

	// Enrich each series with missing volume detection
	for i := range summaries {
		summaries[i] = s.enrichSeriesSummary(summaries[i])
		// Clear items if not requested
		if !opts.IncludeItems {
			summaries[i].Items = nil
		}
	}

	// Filter by status if requested
	if opts.Status != nil {
		filtered := make([]SeriesSummary, 0)
		for _, summary := range summaries {
			if summary.Status == *opts.Status {
				filtered = append(filtered, summary)
			}
		}
		summaries = filtered
	}

	return SeriesListResponse{
		Series: summaries,
	}, nil
}

// GetSeriesByName returns detailed info about a single series with missing volume detection.
func (s *Service) GetSeriesByName(ctx context.Context, name string, ownerID uuid.UUID) (SeriesSummary, error) {
	summary, err := s.repo.GetSeriesByName(ctx, name, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}
	return s.enrichSeriesSummary(summary), nil
}

// UpdateSeriesName renames a series by updating series_name on all matching items.
// Returns the updated series summary.
func (s *Service) UpdateSeriesName(ctx context.Context, oldName, newName string, ownerID uuid.UUID) (SeriesSummary, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	if oldName == "" {
		return SeriesSummary{}, validationErr("series name is required")
	}
	if newName == "" {
		return SeriesSummary{}, validationErr("new series name is required")
	}
	if len(newName) > 200 {
		return SeriesSummary{}, validationErr("series name must be 200 characters or less")
	}

	// Verify series exists
	_, err := s.repo.GetSeriesByName(ctx, oldName, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}

	// Check if target name already exists (case-insensitive).
	existingNames, err := s.repo.ListSeriesNamesByNameCI(ctx, newName, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}
	for _, seriesName := range existingNames {
		if seriesName != oldName {
			return SeriesSummary{}, validationErr("a series with this name already exists")
		}
	}

	_, err = s.repo.UpdateSeriesName(ctx, oldName, newName, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}

	// Return the updated series
	return s.GetSeriesByName(ctx, newName, ownerID)
}

// DeleteSeries removes series association from all items in a series.
// Returns the count of affected items.
func (s *Service) DeleteSeries(ctx context.Context, seriesName string, ownerID uuid.UUID) (int64, error) {
	seriesName = strings.TrimSpace(seriesName)
	if seriesName == "" {
		return 0, validationErr("series name is required")
	}

	// Verify series exists
	_, err := s.repo.GetSeriesByName(ctx, seriesName, ownerID)
	if err != nil {
		return 0, err
	}

	return s.repo.ClearSeriesName(ctx, seriesName, ownerID)
}

// enrichSeriesSummary calculates missing volumes and status for a series.
func (s *Service) enrichSeriesSummary(summary SeriesSummary) SeriesSummary {
	summary.MissingVolumes, summary.MissingCount = s.detectMissingVolumes(summary)
	summary.Status = s.determineSeriesStatus(summary)

	return summary
}

const maxSeriesVolumes = 200

// detectMissingVolumes returns an exact count and at most 200 missing volume numbers.
// Work depends on the owned items and output limit, never the highest volume number.
func (s *Service) detectMissingVolumes(summary SeriesSummary) ([]int, *int) {
	ownedSet := make(map[int]struct{})
	upperBound := 0
	for _, item := range summary.Items {
		if item.VolumeNumber != nil && *item.VolumeNumber > 0 {
			ownedSet[*item.VolumeNumber] = struct{}{}
			upperBound = max(upperBound, *item.VolumeNumber)
		}
	}
	if len(ownedSet) == 0 {
		// Preserve count-based estimates when no volume numbers are available.
		if summary.TotalVolumes == nil {
			return nil, nil
		}
		count := max(0, *summary.TotalVolumes-summary.OwnedCount)
		return nil, &count
	}
	if summary.TotalVolumes != nil {
		upperBound = max(upperBound, *summary.TotalVolumes)
	}
	count := upperBound - len(ownedSet)
	owned := make([]int, 0, len(ownedSet))
	for volume := range ownedSet {
		owned = append(owned, volume)
	}
	slices.Sort(owned)
	missing := make([]int, 0, min(count, maxSeriesVolumes))
	appendGap := func(first, last int) {
		length := min(last-first+1, maxSeriesVolumes-len(missing))
		for offset := 0; offset < length; offset++ {
			missing = append(missing, first+offset)
		}
	}
	next := 1
	for _, volume := range owned {
		appendGap(next, volume-1)
		if len(missing) == maxSeriesVolumes || volume == upperBound {
			return missing, &count
		}
		next = volume + 1
	}
	appendGap(next, upperBound)
	return missing, &count
}

// determineSeriesStatus calculates the completion status of a series.
func (s *Service) determineSeriesStatus(summary SeriesSummary) SeriesStatus {
	if summary.TotalVolumes != nil {
		// Total is known - we can determine complete vs incomplete
		if summary.MissingCount != nil && *summary.MissingCount > 0 {
			return SeriesStatusIncomplete
		}
		return SeriesStatusComplete
	}

	// Total is unknown
	if summary.MissingCount != nil && *summary.MissingCount > 0 {
		// We found gaps via heuristic - incomplete but inferred
		return SeriesStatusIncomplete
	}

	// No gaps found and no total known - status is unknown
	return SeriesStatusUnknown
}
