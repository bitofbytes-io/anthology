package items

import (
	"strings"
)

// NormalizeTitle prepares a title for duplicate comparison by lowercasing and trimming whitespace.
func NormalizeTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

// NormalizeIdentifier strips all non-digit characters from an identifier (ISBN, UPC, EAN).
func NormalizeIdentifier(value string) string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range cleaned {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// isbnMatchKey keeps the digits and any X check digit of an ISBN, uppercased,
// so ISBN-10s ending in X compare correctly. The Postgres repository mirrors
// this by stripping [^0-9Xx] with regexp_replace and applying upper().
func isbnMatchKey(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == 'x' || r == 'X':
			builder.WriteByte('X')
		}
	}
	return builder.String()
}

// normalizeISBN returns the match key for a scanned ISBN, or "" unless it has
// the shape of an ISBN-13 (13 digits) or ISBN-10 (9 digits plus a digit or X).
func normalizeISBN(value string) string {
	key := isbnMatchKey(value)
	switch {
	case len(key) == 13 && !strings.Contains(key, "X"):
		return key
	case len(key) == 10 && !strings.Contains(key[:9], "X"):
		return key
	default:
		return ""
	}
}

// IsISBN reports whether value has the shape of an ISBN-10 or ISBN-13 once
// separators are removed.
func IsISBN(value string) bool {
	return normalizeISBN(value) != ""
}

// isbnLookupKeys returns the match keys a scanned ISBN may be stored under: its
// own normalized form and, when its check digit is valid, the equivalent
// ISBN-13 or ISBN-10, so a scan of either form finds a book saved with the
// other. Only 978-prefixed ISBN-13s have an ISBN-10 form. A code with a wrong
// check digit (a misread) matches only itself: converting it would recompute
// the check digit and could match a different, valid ISBN. It returns nil when
// value is not an ISBN.
func isbnLookupKeys(value string) []string {
	key := normalizeISBN(value)
	switch len(key) {
	case 10:
		if isbn13 := isbn10To13(key); isbn13To10(isbn13) == key {
			return []string{key, isbn13}
		}
		return []string{key}
	case 13:
		if !strings.HasPrefix(key, "978") {
			return []string{key}
		}
		if isbn10 := isbn13To10(key); isbn10To13(isbn10) == key {
			return []string{key, isbn10}
		}
		return []string{key}
	default:
		return nil
	}
}

// isbn10To13 converts a normalized ISBN-10 to its ISBN-13 form by prefixing 978
// and recomputing the check digit.
func isbn10To13(isbn10 string) string {
	body := "978" + isbn10[:9]
	sum := 0
	for i, r := range body {
		digit := int(r - '0')
		if i%2 == 1 {
			digit *= 3
		}
		sum += digit
	}
	return body + string(rune('0'+(10-sum%10)%10))
}

// isbn13To10 converts a normalized 978-prefixed ISBN-13 to its ISBN-10 form by
// dropping the prefix and recomputing the check digit.
func isbn13To10(isbn13 string) string {
	body := isbn13[3:12]
	sum := 0
	for i, r := range body {
		sum += (10 - i) * int(r-'0')
	}
	check := (11 - sum%11) % 11
	if check == 10 {
		return body + "X"
	}
	return body + string(rune('0'+check))
}
