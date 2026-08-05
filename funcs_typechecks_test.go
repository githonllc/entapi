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
