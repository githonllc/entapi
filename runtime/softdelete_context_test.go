package entapi

import (
	"context"
	"testing"
)

// TestSoftDeleteContextSwitchesAreIndependent is the difference from ent's
// published recipe, which uses one key for both: there, a caller who wanted to
// read a tombstone also silently armed a real DELETE.
func TestSoftDeleteContextSwitchesAreIndependent(t *testing.T) {
	base := context.Background()

	if SoftDeletedIncluded(base) || HardDeleteRequested(base) {
		t.Fatal("a plain context reports one of the switches as set")
	}

	soft := WithSoftDeleted(base)
	if !SoftDeletedIncluded(soft) {
		t.Error("WithSoftDeleted did not set its own switch")
	}
	if HardDeleteRequested(soft) {
		t.Error("WithSoftDeleted also armed a hard delete")
	}

	hard := WithHardDelete(base)
	if !HardDeleteRequested(hard) {
		t.Error("WithHardDelete did not set its own switch")
	}
	if SoftDeletedIncluded(hard) {
		t.Error("WithHardDelete also opened up the reads")
	}

	// The parent is untouched: both are per-call, not a mode on the client.
	if SoftDeletedIncluded(base) || HardDeleteRequested(base) {
		t.Error("the parent context was mutated")
	}
}
