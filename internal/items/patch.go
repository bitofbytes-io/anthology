package items

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ItemPatch is a parsed item update body. Keys it contains replace the
// stored values when Service.Update applies it; keys it omits leave them
// unchanged.
type ItemPatch struct {
	fields map[string]json.RawMessage
}

// readOnlyItemKeys are Item's JSON keys that an update never sets, lower-cased
// because encoding/json matches keys case-insensitively.
var readOnlyItemKeys = map[string]bool{"id": true, "createdat": true, "updatedat": true, "shelfplacement": true}

// nullResetItemKeys are the enum fields whose null means "" rather than
// "leave unchanged", which is what encoding/json does for other strings. A
// null itemType therefore fails validation, and a null format, genre or
// readingStatus falls back to its default.
var nullResetItemKeys = map[string]bool{"itemType": true, "format": true, "genre": true, "readingStatus": true}

// editableItemKeys are the exact JSON keys of Item's editable fields.
var editableItemKeys = func() map[string]bool {
	keys := map[string]bool{}
	itemType := reflect.TypeFor[Item]()
	for i := range itemType.NumField() {
		name, _, _ := strings.Cut(itemType.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" && !readOnlyItemKeys[strings.ToLower(name)] {
			keys[name] = true
		}
	}
	return keys
}()

// ParseItemPatch parses an item update body: a JSON object, or null, keyed by
// Item's JSON field names.
//
// A null clears a pointer field such as releaseYear or readAt, leaves a plain
// string field such as title unchanged, and resets itemType, format, genre and
// readingStatus to "". Keys must match exactly; read-only keys (id, createdAt,
// updatedAt, shelfPlacement) and unknown keys are ignored.
//
// It fails on malformed JSON or a value of the wrong type. Values are
// type-checked the way encoding/json would decode them into an Item, which
// ignores key case, so {"TITLE": 5} fails even though "TITLE" is ignored.
func ParseItemPatch(body []byte) (ItemPatch, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return ItemPatch{}, err
	}
	fields := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		if readOnlyItemKeys[strings.ToLower(key)] {
			delete(raw, key)
			continue
		}
		if !editableItemKeys[key] {
			continue
		}
		if nullResetItemKeys[key] && string(value) == "null" {
			value = json.RawMessage(`""`)
		}
		fields[key] = value
	}
	if err := decodeFields(raw, &Item{}); err != nil {
		return ItemPatch{}, err
	}
	return ItemPatch{fields: fields}, nil
}

// applyTo decodes the patch onto item.
func (p ItemPatch) applyTo(item *Item) error {
	// Decoding writes through non-nil pointers, which item may share with the
	// repository's copy, so give each pointer field its own value first.
	value := reflect.ValueOf(item).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		if field.Kind() == reflect.Pointer && !field.IsNil() {
			fresh := reflect.New(field.Type().Elem())
			fresh.Elem().Set(field.Elem())
			field.Set(fresh)
		}
	}
	if err := decodeFields(p.fields, item); err != nil {
		return fmt.Errorf("apply item patch: %w", err)
	}
	return nil
}

func decodeFields(fields map[string]json.RawMessage, dst *Item) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
