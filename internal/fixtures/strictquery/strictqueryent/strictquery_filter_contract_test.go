// This file is HAND-WRITTEN. Generated filter behavior is asserted through
// ParseRecordQuery and the generated list-handler boundary.
package strictqueryent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	entapi "github.com/githonllc/entapi/runtime"
)

func TestStrictUnknownOperatorPrefixIsValidationError(t *testing.T) {
	_, _, err := ParseRecordQuery(url.Values{"key": {"foo:bar"}})
	if !errors.Is(err, entapi.ErrValidation) {
		t.Fatalf("ParseRecordQuery(key=foo:bar) error = %v, want ErrValidation", err)
	}
	for _, want := range []string{`field "key"`, `value "foo:bar"`, `prefix "foo"`, `"eq:"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestStrictExplicitEqualityEscapesColon(t *testing.T) {
	filter, _, err := ParseRecordQuery(url.Values{"key": {"eq:foo:bar"}})
	if err != nil {
		t.Fatalf("ParseRecordQuery(key=eq:foo:bar): %v", err)
	}
	if want := []string{"foo:bar"}; !reflect.DeepEqual(filter.Key, want) {
		t.Fatalf("Key = %#v, want %#v", filter.Key, want)
	}
}

func TestStrictBareRFC3339TimestampIsBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/records?created_at=2026-01-01T10:30:00Z", nil)

	(&APIHandler{}).handleListRecords(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bare RFC-3339 timestamp status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestStrictKnownButDisallowedOperatorKeepsOldMessage(t *testing.T) {
	_, _, err := ParseRecordQuery(url.Values{"key": {"like:scan"}})
	if !errors.Is(err, entapi.ErrValidation) {
		t.Fatalf("ParseRecordQuery(key=like:scan) error = %v, want ErrValidation", err)
	}
	want := `validation failed: field "key" value "like:scan" uses operator "like"; legal operators: eq, ne, in, not_in, gt, ge, lt, le, from, to, between`
	if err.Error() != want {
		t.Fatalf("error = %q, want unchanged message %q", err, want)
	}
}
