package items

import (
	"strconv"
	"testing"
)

func TestSeriesVolumeLimit(t *testing.T) {
	for _, n := range []int{200, 201} {
		for _, field := range []string{"volume", "total"} {
			t.Run(field+strconv.Itoa(n), func(t *testing.T) {
				var volume, total *int
				if field == "volume" {
					volume = &n
				} else {
					total = &n
				}
				_, _, _, err := normalizeSeriesFields(ItemTypeBook, "Series", volume, total)
				if (err != nil) != (n > 200) {
					t.Fatalf("value %d: %v", n, err)
				}
			})
		}
	}
}

func TestLegacyMissingVolumesAreBounded(t *testing.T) {
	large, one := 2000000000, 1
	for _, volume := range []*int{&one, &large} {
		missing, count := (&Service{}).detectMissingVolumes(SeriesSummary{TotalVolumes: &large, Items: []Item{{VolumeNumber: volume}}})
		if len(missing) > 200 || count == nil || *count != large-1 {
			t.Fatalf("unbounded missing volumes: %v", missing)
		}
	}
}

func TestLegacyDeclaredTotalKeepsExactMissingCount(t *testing.T) {
	total := 250
	summary := SeriesSummary{TotalVolumes: &total}
	for volume := 1; volume <= 200; volume++ {
		summary.Items = append(summary.Items, Item{VolumeNumber: &volume})
	}
	// Duplicate copies must not inflate the distinct volume count.
	summary.Items = append(summary.Items, summary.Items[0])
	summary.OwnedCount = len(summary.Items)
	enriched := (&Service{}).enrichSeriesSummary(summary)
	if enriched.MissingCount == nil || *enriched.MissingCount != 50 || enriched.Status != SeriesStatusIncomplete {
		t.Fatalf("incorrect legacy status: %+v", enriched)
	}
	if len(enriched.MissingVolumes) != 50 || enriched.MissingVolumes[0] != 201 || enriched.MissingVolumes[49] != 250 {
		t.Fatalf("wrong missing list: %v", enriched.MissingVolumes)
	}
}

func TestLegacyInferredHugeGapKeepsExactMissingCount(t *testing.T) {
	large := 2000000000
	summary := SeriesSummary{}
	for volume := 1; volume <= 200; volume++ {
		summary.Items = append(summary.Items, Item{VolumeNumber: &volume})
	}
	summary.Items = append(summary.Items, Item{VolumeNumber: &large}, Item{VolumeNumber: &large})
	enriched := (&Service{}).enrichSeriesSummary(summary)
	if enriched.MissingCount == nil || *enriched.MissingCount != large-201 || enriched.Status != SeriesStatusIncomplete {
		t.Fatalf("incorrect inferred status: %+v", enriched)
	}
	if len(enriched.MissingVolumes) != 200 || enriched.MissingVolumes[0] != 201 || enriched.MissingVolumes[199] != 400 {
		t.Fatalf("wrong bounded list: %v", enriched.MissingVolumes)
	}
}
