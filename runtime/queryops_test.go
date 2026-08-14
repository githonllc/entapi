package entapi

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
)

// parseIntValue mirrors a generated parse function for an int field named "n".
func parseIntValue(raw, whole string) (int, error) {
	parsed, err := strconv.ParseInt(raw, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("%w: field %q value %q is not a valid int: %v", ErrValidation, "n", whole, err)
	}
	return int(parsed), nil
}

func parseStringValue(raw, whole string) (string, error) {
	return raw, nil
}

// intTable builds the full ordered-type dispatch table over the given slots,
// the way filter.tmpl renders one for an int field.
type intSlots struct {
	eq, neq, gt, gte, lt, lte []int
	in, notIn                 [][]int
	isNull                    []bool
}

func intTable(s *intSlots) []QueryOp[int] {
	return []QueryOp[int]{
		{Prefix: "eq", Kind: OpKindValue, One: &s.eq},
		{Prefix: "ne", Kind: OpKindValue, One: &s.neq},
		{Prefix: "in", Kind: OpKindList, Many: &s.in},
		{Prefix: "not_in", Kind: OpKindList, Many: &s.notIn},
		{Prefix: "gt", Kind: OpKindValue, One: &s.gt},
		{Prefix: "ge", Kind: OpKindValue, One: &s.gte},
		{Prefix: "lt", Kind: OpKindValue, One: &s.lt},
		{Prefix: "le", Kind: OpKindValue, One: &s.lte},
		{Prefix: "is_null", Kind: OpKindFlag, Null: &s.isNull, BoolValue: true},
		{Prefix: "not_null", Kind: OpKindFlag, Null: &s.isNull},
		{Prefix: "from", Kind: OpKindValue, One: &s.gte},
		{Prefix: "to", Kind: OpKindValue, One: &s.lte},
		{Prefix: "between", Kind: OpKindRange, One: &s.gte, Second: &s.lte},
	}
}

const intLegal = "eq, ne, in, not_in, gt, ge, lt, le, is_null, not_null, from, to, between"

func mustParse(t *testing.T, s *intSlots, strict bool, values ...string) {
	t.Helper()
	if err := ParseFieldValues("n", values, strict, intLegal, parseIntValue, intTable(s)); err != nil {
		t.Fatalf("ParseFieldValues(%q) = %v, want nil", values, err)
	}
}

func wantParseError(t *testing.T, strict bool, want string, values ...string) {
	t.Helper()
	var s intSlots
	err := ParseFieldValues("n", values, strict, intLegal, parseIntValue, intTable(&s))
	if err == nil {
		t.Fatalf("ParseFieldValues(%q) = nil, want error %q", values, want)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("ParseFieldValues(%q) error %v does not wrap ErrValidation", values, err)
	}
	if err.Error() != want {
		t.Fatalf("ParseFieldValues(%q) error =\n  %q\nwant\n  %q", values, err.Error(), want)
	}
}

func TestParseFieldValues_ValueOperators(t *testing.T) {
	var s intSlots
	mustParse(t, &s, false, "5", "eq:6", "ne:7", "gt:1", "ge:2", "lt:3", "le:4", "from:8", "to:9")
	if got, want := fmt.Sprint(s.eq), "[5 6]"; got != want {
		t.Errorf("eq slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.neq), "[7]"; got != want {
		t.Errorf("ne slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.gt), "[1]"; got != want {
		t.Errorf("gt slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.gte), "[2 8]"; got != want {
		t.Errorf("ge/from slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.lt), "[3]"; got != want {
		t.Errorf("lt slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.lte), "[4 9]"; got != want {
		t.Errorf("le/to slot = %s, want %s", got, want)
	}
}

func TestParseFieldValues_ListOperators(t *testing.T) {
	var s intSlots
	mustParse(t, &s, false, "in:1,2,3", "not_in:4,5")
	if got, want := fmt.Sprint(s.in), "[[1 2 3]]"; got != want {
		t.Errorf("in slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.notIn), "[[4 5]]"; got != want {
		t.Errorf("not_in slot = %s, want %s", got, want)
	}
}

func TestParseFieldValues_FlagOperators(t *testing.T) {
	var s intSlots
	mustParse(t, &s, false, "is_null:", "not_null:")
	if got, want := fmt.Sprint(s.isNull), "[true false]"; got != want {
		t.Errorf("is_null slot = %s, want %s", got, want)
	}
}

func TestParseFieldValues_FlagWithValueFails(t *testing.T) {
	wantParseError(t, false,
		`validation failed: field "n" value "is_null:x" uses "is_null" with a value; legal operators: `+intLegal,
		"is_null:x")
}

func TestParseFieldValues_Between(t *testing.T) {
	var s intSlots
	mustParse(t, &s, false, "between:1,5")
	if got, want := fmt.Sprint(s.gte), "[1]"; got != want {
		t.Errorf("between lower slot = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(s.lte), "[5]"; got != want {
		t.Errorf("between upper slot = %s, want %s", got, want)
	}
}

func TestParseFieldValues_BetweenWrongParts(t *testing.T) {
	wantParseError(t, false,
		`validation failed: field "n" value "between:1" uses between with 1 parts; exactly two are required`,
		"between:1")
	wantParseError(t, false,
		`validation failed: field "n" value "between:1,2,3" uses between with 3 parts; exactly two are required`,
		"between:1,2,3")
}

func TestParseFieldValues_EmptyValueSkipped(t *testing.T) {
	var s intSlots
	mustParse(t, &s, false, "", "5")
	if got, want := fmt.Sprint(s.eq), "[5]"; got != want {
		t.Errorf("eq slot = %s, want %s", got, want)
	}
}

func TestParseFieldValues_KnownOperatorNotAllowed(t *testing.T) {
	// An enum-shaped table permits only eq/ne/in/not_in; gt is in the global
	// vocabulary, so it must be refused with the field's legal list — in both
	// modes, because KnownQueryOperator is checked before the strict branch.
	var eq, neq []string
	var in, notIn [][]string
	table := []QueryOp[string]{
		{Prefix: "eq", Kind: OpKindValue, One: &eq},
		{Prefix: "ne", Kind: OpKindValue, One: &neq},
		{Prefix: "in", Kind: OpKindList, Many: &in},
		{Prefix: "not_in", Kind: OpKindList, Many: &notIn},
	}
	want := `validation failed: field "status" value "gt:x" uses operator "gt"; legal operators: eq, ne, in, not_in`
	for _, strict := range []bool{false, true} {
		err := ParseFieldValues("status", []string{"gt:x"}, strict, "eq, ne, in, not_in", parseStringValue, table)
		if err == nil || err.Error() != want {
			t.Fatalf("strict=%v: error = %v, want %q", strict, err, want)
		}
	}
}

func TestParseFieldValues_UnknownPrefixLenientFallsBackToWholeValue(t *testing.T) {
	var eq []string
	table := []QueryOp[string]{{Prefix: "eq", Kind: OpKindValue, One: &eq}}
	if err := ParseFieldValues("name", []string{"foo:bar"}, false, "eq", parseStringValue, table); err != nil {
		t.Fatalf("lenient unknown prefix: %v", err)
	}
	// The whole value, colon included, is the equality operand.
	if got, want := fmt.Sprint(eq), "[foo:bar]"; got != want {
		t.Errorf("eq slot = %s, want %s", got, want)
	}
}

func TestParseFieldValues_UnknownPrefixStrictFails(t *testing.T) {
	wantParseError(t, true,
		`validation failed: field "n" value "foo:bar" uses unknown operator prefix "foo"; legal operators: `+intLegal+`; prefix the value with "eq:" to filter for a literal value containing a colon`,
		"foo:bar")
}

func TestParseFieldValues_EqAbsentFromTable(t *testing.T) {
	// A table with no eq row (a null-only field): a bare value resolves to eq,
	// which is a known operator outside the field's set.
	var isNull []bool
	table := []QueryOp[int]{
		{Prefix: "is_null", Kind: OpKindFlag, Null: &isNull, BoolValue: true},
		{Prefix: "not_null", Kind: OpKindFlag, Null: &isNull},
	}
	want := `validation failed: field "meta" value "5" uses operator "eq"; legal operators: is_null, not_null`
	err := ParseFieldValues("meta", []string{"5"}, false, "is_null, not_null", parseIntValue, table)
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestParseFieldValues_ParseErrorPropagates(t *testing.T) {
	var s intSlots
	err := ParseFieldValues("n", []string{"gt:abc"}, false, intLegal, parseIntValue, intTable(&s))
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want wrapped ErrValidation", err)
	}
	want := `validation failed: field "n" value "gt:abc" is not a valid int: strconv.ParseInt: parsing "abc": invalid syntax`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseFieldValues_ListParseErrorPropagates(t *testing.T) {
	var s intSlots
	err := ParseFieldValues("n", []string{"in:1,x"}, false, intLegal, parseIntValue, intTable(&s))
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want wrapped ErrValidation", err)
	}
	want := `validation failed: field "n" value "in:1,x" is not a valid int: strconv.ParseInt: parsing "x": invalid syntax`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if len(s.in) != 0 {
		t.Errorf("in slot = %v, want empty after failed parse", s.in)
	}
}

func TestParseFieldValues_RangeParseErrorPropagates(t *testing.T) {
	// between parses both parts before appending either, so a failure in the
	// lower or the upper part surfaces that part's error unchanged and leaves
	// both slots empty.
	for _, tc := range []struct {
		value string
		want  string
	}{
		{
			value: "between:x,2",
			want:  `validation failed: field "n" value "between:x,2" is not a valid int: strconv.ParseInt: parsing "x": invalid syntax`,
		},
		{
			value: "between:1,x",
			want:  `validation failed: field "n" value "between:1,x" is not a valid int: strconv.ParseInt: parsing "x": invalid syntax`,
		},
	} {
		var s intSlots
		err := ParseFieldValues("n", []string{tc.value}, false, intLegal, parseIntValue, intTable(&s))
		if err == nil || !errors.Is(err, ErrValidation) {
			t.Fatalf("%s: error = %v, want wrapped ErrValidation", tc.value, err)
		}
		if err.Error() != tc.want {
			t.Fatalf("%s: error = %q, want %q", tc.value, err.Error(), tc.want)
		}
		if len(s.gte) != 0 || len(s.lte) != 0 {
			t.Errorf("%s: ge/le slots = %v/%v, want both empty after failed parse", tc.value, s.gte, s.lte)
		}
	}
}
