package catalog

import "testing"

func TestMapCategoriesToGenreReturnsEmptyWhenCategoriesEmpty(t *testing.T) {
	t.Parallel()
	if got := MapCategoriesToGenre(nil); got != "" {
		t.Fatalf("expected empty genre, got %q", got)
	}
}

func TestMapCategoriesToGenreReturnsEmptyWhenNoMatch(t *testing.T) {
	t.Parallel()
	if got := MapCategoriesToGenre([]string{"Totally Unrelated Category"}); got != "" {
		t.Fatalf("expected empty genre when no mapping matches, got %q", got)
	}
}

func TestMapCategoriesToGenreReturnsReferenceOtherWhenMatched(t *testing.T) {
	t.Parallel()
	if got := MapCategoriesToGenre([]string{"Reference"}); got != genreReferenceOther {
		t.Fatalf("expected %q, got %q", genreReferenceOther, got)
	}
}
