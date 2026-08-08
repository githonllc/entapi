package entapi

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSplitOpSplitsOnTheFirstColon(t *testing.T) {
	tests := []struct {
		value               string
		wantOp, wantOperand string
		wantExplicit        bool
	}{
		{value: "plain", wantOperand: "plain"},
		{value: "eq:", wantOp: "eq", wantExplicit: true},
		{value: "prefix:12:30", wantOp: "prefix", wantOperand: "12:30", wantExplicit: true},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			op, operand, explicit := SplitOp(tc.value)
			if op != tc.wantOp || operand != tc.wantOperand || explicit != tc.wantExplicit {
				t.Fatalf("SplitOp(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.value, op, operand, explicit, tc.wantOp, tc.wantOperand, tc.wantExplicit)
			}
		})
	}
}

func TestKnownQueryOperatorUsesTheGlobalWireVocabulary(t *testing.T) {
	for _, op := range []string{"eq", "ne", "gt", "ge", "lt", "le", "in", "not_in", "like", "ilike", "prefix", "suffix", "is_null", "not_null", "from", "to", "between"} {
		if !KnownQueryOperator(op) {
			t.Errorf("KnownQueryOperator(%q) = false", op)
		}
	}
	for _, op := range []string{"", "_ieq", "gte", "contains", "bogus"} {
		if KnownQueryOperator(op) {
			t.Errorf("KnownQueryOperator(%q) = true", op)
		}
	}
}

func TestParseSortSpecsParsesTheWireGrammar(t *testing.T) {
	got, err := ParseSortSpecs("created_at:desc,id,title")
	if err != nil {
		t.Fatalf("ParseSortSpecs: %v", err)
	}
	want := []SortSpec{{Key: "created_at", Desc: true}, {Key: "id"}, {Key: "title"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSortSpecs = %+v, want %+v", got, want)
	}

	for _, value := range []string{"created_at:sideways", "created_at:DESC", "created_at:asc:extra", "created_at,"} {
		_, err := ParseSortSpecs(value)
		if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), value) {
			t.Errorf("ParseSortSpecs(%q) error = %v, want ErrValidation naming the value", value, err)
		}
	}
}

func TestReservedQueryValueRequiresOneSpelling(t *testing.T) {
	for _, key := range []string{"_sort", "_page", "_size", "_q"} {
		value, recognized, err := ReservedQueryValue(key, []string{"a"})
		if err != nil || !recognized || value != "a" {
			t.Errorf("ReservedQueryValue(%q) = (%q, %v, %v)", key, value, recognized, err)
		}
		_, recognized, err = ReservedQueryValue(key, []string{"a", "b"})
		if !recognized || !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), key) {
			t.Errorf("duplicate %s error = %v, recognized=%v", key, err, recognized)
		}
	}
	if _, recognized, err := ReservedQueryValue("_other", []string{"a"}); err != nil || recognized {
		t.Fatalf("unknown reserved name recognized=%v, err=%v", recognized, err)
	}
}

func TestParsePageParamRejectsInvalidWireValues(t *testing.T) {
	for _, tc := range []struct {
		name, value, message string
	}{
		{name: "_page", value: "abc"},
		{name: "_page", value: "0"},
		{name: "_page", value: "-1"},
		{name: "_size", value: "abc"},
		{name: "_size", value: "0", message: "count-only"},
		{name: "_size", value: "-1"},
	} {
		_, err := ParsePageParam(tc.name, tc.value)
		if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), tc.name) || !strings.Contains(err.Error(), tc.value) {
			t.Errorf("ParsePageParam(%q, %q) error = %v", tc.name, tc.value, err)
		}
		if tc.message != "" && !strings.Contains(err.Error(), tc.message) {
			t.Errorf("ParsePageParam(%q, %q) error %q does not mention %q", tc.name, tc.value, err, tc.message)
		}
	}
	if got, err := ParsePageParam("_size", "1001"); err != nil || got != 1001 {
		t.Fatalf("oversized _size must reach Limit for clamping: got %d, err=%v", got, err)
	}
}
