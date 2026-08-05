package entdomain

import (
	"testing"
)

func TestIsTimeField(t *testing.T) {
	timeField := newTimeField("created_at", nil)
	stringField := newStringField("name", nil)
	intField := newIntField("age", nil)

	if !isTimeField(timeField) {
		t.Error("expected time field to return true")
	}
	if isTimeField(stringField) {
		t.Error("expected string field to return false")
	}
	if isTimeField(intField) {
		t.Error("expected int field to return false")
	}
}

func TestHasTimeFields(t *testing.T) {
	df := ptr(DefaultField())

	// Type with time fields
	withTime := newTestType("User",
		newStringField("name", df),
		newTimeField("created_at", df),
	)
	if !hasTimeFields(withTime) {
		t.Error("expected hasTimeFields to return true")
	}

	// Type without time fields
	withoutTime := newTestType("User",
		newStringField("name", df),
		newIntField("age", df),
	)
	if hasTimeFields(withoutTime) {
		t.Error("expected hasTimeFields to return false")
	}

	// Empty type
	empty := newTestType("Empty")
	if hasTimeFields(empty) {
		t.Error("expected hasTimeFields to return false for empty type")
	}
}
