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
		missing := (&Service{}).detectMissingVolumes(SeriesSummary{TotalVolumes: &large, Items: []Item{{VolumeNumber: volume}}})
		if len(missing) > 200 || missing[len(missing)-1] > 200 {
			t.Fatalf("unbounded missing volumes: %v", missing)
		}
	}
}
