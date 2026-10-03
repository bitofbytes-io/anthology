package items

import (
	"slices"
	"testing"
)

func TestISBNConversions(t *testing.T) {
	pairs := []struct{ isbn10, isbn13 string }{
		{"0306406152", "9780306406157"},
		{"080442957X", "9780804429573"},
		{"0000000000", "9780000000002"},
	}
	for _, pair := range pairs {
		if got := isbn10To13(pair.isbn10); got != pair.isbn13 {
			t.Errorf("isbn10To13(%s) = %s, want %s", pair.isbn10, got, pair.isbn13)
		}
		if got := isbn13To10(pair.isbn13); got != pair.isbn10 {
			t.Errorf("isbn13To10(%s) = %s, want %s", pair.isbn13, got, pair.isbn10)
		}
	}
}

func TestISBNLookupKeys(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"0-306-40615-2", []string{"0306406152", "9780306406157"}},
		{"978-0-306-40615-7", []string{"9780306406157", "0306406152"}},
		{"0-8044-2957-x", []string{"080442957X", "9780804429573"}},
		{"9791234567896", []string{"9791234567896"}},
		{"012345678905", nil},
		{"not-an-isbn", nil},
		{"", nil},
	}
	for _, tt := range tests {
		if got := isbnLookupKeys(tt.input); !slices.Equal(got, tt.want) {
			t.Errorf("isbnLookupKeys(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsISBN(t *testing.T) {
	for _, valid := range []string{"9780306406157", "0-306-40615-2", "080442957x"} {
		if !IsISBN(valid) {
			t.Errorf("IsISBN(%q) = false, want true", valid)
		}
	}
	for _, invalid := range []string{"", "012345678905", "08044X2957", "abc"} {
		if IsISBN(invalid) {
			t.Errorf("IsISBN(%q) = true, want false", invalid)
		}
	}
}
