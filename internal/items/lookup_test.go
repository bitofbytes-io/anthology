package items

import (
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The Great Gatsby", "the great gatsby"},
		{"  The Great Gatsby  ", "the great gatsby"},
		{"THE GREAT GATSBY", "the great gatsby"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeTitle(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"9780132350884", "9780132350884"},
		{"978-0132350884", "9780132350884"},
		{"978 0132350884", "9780132350884"},
		{"978-0-13235-088-4", "9780132350884"},
		{"", ""},
		{"   ", ""},
		{"abc", ""},
		{"0-13235-0882", "0132350882"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeIdentifier(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
