// This file is HAND-WRITTEN, like presence/presenceent/account_presence_test.go,
// and unlike everything else under a fixture's ent/ directory. It carries no
// DO NOT EDIT header on purpose.
//
// It is in package fieldshapesent because the contract it pins is behavioural:
// TestCodegenFixtures proves the patch value readers (#113) COMPILE for a field
// whose Go type is itself nillable, which says nothing about what they answer
// there. A []string is the shape where "no value" and "the zero value" are two
// different things the reader must not confuse — nil, an empty slice and an
// absent key are three distinct answers, and only the comma-ok bit separates
// them.
package fieldshapesent

import (
	"encoding/json"
	"testing"
)

// decodeJSONWidgetPatch unmarshals a patch request the way a handler would, so
// presence comes from the payload rather than from a struct literal.
func decodeJSONWidgetPatch(t *testing.T, body string) *ValidJSONWidgetPatchRequest {
	t.Helper()
	var r JSONWidgetPatchRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	v, err := r.Validate()
	if err != nil {
		t.Fatalf("validating %s: %v", body, err)
	}
	return v
}

// TestPatchValueReaderOnANillableGoType is #113 against the field shape that
// breaks a `var zero T` written as `""` or `0`: the value type is a slice, so
// the reader's zero has to be spelled generically.
//
// The three answers are all reachable and all different:
//
//	{"tags":null}  present, no value  -> (nil, false), and Apply clears it
//	{"tags":[]}    present, a value   -> (non-nil empty slice, true)
//	absent         neither            -> (nil, false)
//
// The middle row is the one a nil check alone gets wrong: an empty slice IS the
// value the caller sent, and it is not the same request as a null.
func TestPatchValueReaderOnANillableGoType(t *testing.T) {
	t.Run("an explicit null carries no value", func(t *testing.T) {
		v := decodeJSONWidgetPatch(t, `{"tags":null}`)
		if !v.HasTags() {
			t.Fatal(`HasTags() = false for {"tags":null}: an explicit null is present`)
		}
		got, ok := v.Tags()
		if ok {
			t.Errorf("Tags() = (%v, true) for an explicit null, want ok false: Apply clears the field", got)
		}
		if got != nil {
			t.Errorf("Tags() = %v, want nil for the zero value of []string", got)
		}
	})

	t.Run("an empty slice is a value", func(t *testing.T) {
		v := decodeJSONWidgetPatch(t, `{"tags":[]}`)
		got, ok := v.Tags()
		if !ok {
			t.Fatalf("Tags() = (%v, false) for an empty array; the caller sent a value", got)
		}
		if got == nil {
			t.Error("Tags() = nil for an empty array, which is the answer an explicit null gets")
		}
		if len(got) != 0 {
			t.Errorf("Tags() = %v, want an empty slice", got)
		}
	})

	t.Run("absent is neither", func(t *testing.T) {
		v := decodeJSONWidgetPatch(t, `{}`)
		if v.HasTags() {
			t.Error("HasTags() = true for {}")
		}
		if got, ok := v.Tags(); ok || got != nil {
			t.Errorf("Tags() = (%v, %v), want (nil, false)", got, ok)
		}
	})

	t.Run("a hand-built request reads as absent", func(t *testing.T) {
		tags := []string{"a"}
		r := &JSONWidgetPatchRequest{Tags: &tags}
		v, err := r.Validate()
		if err != nil {
			t.Fatalf("validating a hand-built patch request: %v", err)
		}
		if v.HasTags() {
			t.Error("HasTags() = true on a request that never went through UnmarshalJSON")
		}
		if got, ok := v.Tags(); ok {
			t.Errorf("Tags() = (%v, %v) on a hand-built request, want (nil, false): Apply writes nothing for it", got, ok)
		}
	})
}
