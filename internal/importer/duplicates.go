package importer

import (
	"fmt"
	"strings"

	"anthology/internal/items"
)

// Duplicate fields, in the order they are checked.
const (
	fieldTitle  = "title"
	fieldISBN13 = "isbn13"
	fieldISBN10 = "isbn10"
)

// duplicateKey is one exact-match key: a case-insensitive trimmed title, or
// the digits of an ISBN-13 or ISBN-10 compared only with the same field.
type duplicateKey struct {
	field string
	value string
}

func (k duplicateKey) String() string {
	return k.field + ":" + k.value
}

// duplicateKeys returns the keys a CSV row or library item is matched on, in
// check order. Empty values produce no key.
func duplicateKeys(title, isbn13, isbn10 string) []duplicateKey {
	keys := make([]duplicateKey, 0, 3)
	if value := strings.ToLower(strings.TrimSpace(title)); value != "" {
		keys = append(keys, duplicateKey{fieldTitle, value})
	}
	if value := items.NormalizeIdentifier(isbn13); value != "" {
		keys = append(keys, duplicateKey{fieldISBN13, value})
	}
	if value := items.NormalizeIdentifier(isbn10); value != "" {
		keys = append(keys, duplicateKey{fieldISBN10, value})
	}
	return keys
}

func inputKeys(input items.CreateItemInput) []duplicateKey {
	return duplicateKeys(input.Title, input.ISBN13, input.ISBN10)
}

// keyStrings renders keys for the preview response, where the browser uses
// them to recompute in-file duplicates after a catalog match is chosen.
func keyStrings(keys []duplicateKey) []string {
	out := make([]string, len(keys))
	for idx, key := range keys {
		out[idx] = key.String()
	}
	return out
}

// duplicateSource records what first claimed a key: a library item, or a row
// of the current import (item is nil).
type duplicateSource struct {
	field string
	item  *items.Item
	row   int
}

// duplicateTracker applies the importer's duplicate rule: a row duplicates an
// existing item, or an earlier row of the same import, when its trimmed title
// matches case-insensitively or its ISBN-13 or ISBN-10 digits match the same
// field. Nothing is fuzzy, and it only knows the items it was given.
type duplicateTracker struct {
	known map[duplicateKey]duplicateSource
}

func newDuplicateTracker(existing []items.Item) *duplicateTracker {
	tracker := &duplicateTracker{known: map[duplicateKey]duplicateSource{}}
	for idx := range existing {
		item := &existing[idx]
		for _, key := range duplicateKeys(item.Title, item.ISBN13, item.ISBN10) {
			tracker.store(key, duplicateSource{field: key.field, item: item})
		}
	}
	return tracker
}

// store keeps the first source for each key.
func (t *duplicateTracker) store(key duplicateKey, source duplicateSource) {
	if _, ok := t.known[key]; !ok {
		t.known[key] = source
	}
}

// Matches returns the source of every key input shares with the tracker, in
// check order.
func (t *duplicateTracker) Matches(input items.CreateItemInput) []duplicateSource {
	var matches []duplicateSource
	for _, key := range inputKeys(input) {
		if source, ok := t.known[key]; ok {
			matches = append(matches, source)
		}
	}
	return matches
}

// Check reports whether input is a duplicate and, if so, the field that
// matched first.
func (t *duplicateTracker) Check(input items.CreateItemInput) (string, bool) {
	matches := t.Matches(input)
	if len(matches) == 0 {
		return "", false
	}
	return fmt.Sprintf("duplicate %s", matches[0].field), true
}

// Add records input as imported without a row number.
func (t *duplicateTracker) Add(input items.CreateItemInput) {
	t.AddRow(input, 0)
}

// AddRow records input as saved (or, in a preview, as going to be saved) from
// the given CSV row.
func (t *duplicateTracker) AddRow(input items.CreateItemInput, row int) {
	for _, key := range inputKeys(input) {
		t.store(key, duplicateSource{field: key.field, row: row})
	}
}
