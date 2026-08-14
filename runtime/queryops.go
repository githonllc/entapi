package entapi

import (
	"fmt"
	"strings"
)

// OpKind selects the dispatch shape of one QueryOp table row.
type OpKind int

const (
	// OpKindValue parses the operand as one value and appends it to One.
	OpKindValue OpKind = iota
	// OpKindList splits the operand on commas, parses each part and appends the
	// whole slice to Many as one entry.
	OpKindList
	// OpKindFlag is the valueless is_null/not_null pair: the operand must be
	// empty and BoolValue is appended to Null.
	OpKindFlag
	// OpKindRange is between: exactly two comma-separated parts, the lower
	// appended to One and the upper to Second.
	OpKindRange
)

// QueryOp is one operator row of a generated field's dispatch table. Exactly
// one slot pointer group is set per row, matching Kind. The slots are typed
// pointers into the generated filter struct, so a missing or wrong-typed
// struct field is a compile error in the generated code, and dispatch involves
// no reflection and no string-keyed runtime lookup of the predicate target.
// The pairing between Kind and which slot is set is NOT checked by the
// compiler: it is a contract upheld by filter.tmpl, the sole construction
// site. ParseFieldValues catches only a Kind outside the declared range, which
// it reports as a plain error rather than a validation failure; a row whose
// Kind is one of the four but whose matching slot pointer is nil panics on the
// nil dereference.
type QueryOp[T any] struct {
	Prefix    string
	Kind      OpKind
	One       *[]T    // OpKindValue target; OpKindRange lower target
	Second    *[]T    // OpKindRange upper target
	Many      *[][]T  // OpKindList target
	Null      *[]bool // OpKindFlag target
	BoolValue bool    // OpKindFlag value
}

// ParseFieldValues runs the lexical dispatch loop for one field's URL values.
// Generated code owns the table — the field-local operator permission set,
// the conversion function and the predicate slots; this function owns only
// the mechanics shared by every field: the operator split, the eq fallback,
// comma splitting and validation-error wrapping. field is the wire (storage)
// key, legal is the pre-joined legal-operator list used verbatim in error
// messages, and strict mirrors WithStrictQueryOperators at generation time.
func ParseFieldValues[T any](field string, values []string, strict bool, legal string,
	parse func(raw, whole string) (T, error), ops []QueryOp[T]) error {
	for _, whole := range values {
		if whole == "" {
			continue
		}
		op, raw, explicit := SplitOp(whole)
		if !explicit {
			op = "eq"
		}
		row := findQueryOp(ops, op)
		if row == nil {
			if KnownQueryOperator(op) {
				return fmt.Errorf("%w: field %q value %q uses operator %q; legal operators: %s", ErrValidation, field, whole, op, legal)
			}
			if strict {
				return fmt.Errorf("%w: field %q value %q uses unknown operator prefix %q; legal operators: %s; prefix the value with \"eq:\" to filter for a literal value containing a colon", ErrValidation, field, whole, op, legal)
			}
			op, raw = "eq", whole
			if row = findQueryOp(ops, op); row == nil {
				return fmt.Errorf("%w: field %q value %q uses operator %q; legal operators: %s", ErrValidation, field, whole, op, legal)
			}
		}
		switch row.Kind {
		case OpKindFlag:
			if raw != "" {
				return fmt.Errorf("%w: field %q value %q uses %q with a value; legal operators: %s", ErrValidation, field, whole, op, legal)
			}
			*row.Null = append(*row.Null, row.BoolValue)
		case OpKindList:
			parts := strings.Split(raw, ",")
			parsedValues := make([]T, 0, len(parts))
			for _, part := range parts {
				parsed, err := parse(part, whole)
				if err != nil {
					return err
				}
				parsedValues = append(parsedValues, parsed)
			}
			*row.Many = append(*row.Many, parsedValues)
		case OpKindRange:
			parts := strings.Split(raw, ",")
			if len(parts) != 2 {
				return fmt.Errorf("%w: field %q value %q uses between with %d parts; exactly two are required", ErrValidation, field, whole, len(parts))
			}
			lower, err := parse(parts[0], whole)
			if err != nil {
				return err
			}
			upper, err := parse(parts[1], whole)
			if err != nil {
				return err
			}
			*row.One = append(*row.One, lower)
			*row.Second = append(*row.Second, upper)
		case OpKindValue:
			parsed, err := parse(raw, whole)
			if err != nil {
				return err
			}
			*row.One = append(*row.One, parsed)
		default:
			return fmt.Errorf("entapi: field %q query operator %q has invalid OpKind %d", field, row.Prefix, row.Kind)
		}
	}
	return nil
}

func findQueryOp[T any](ops []QueryOp[T], prefix string) *QueryOp[T] {
	for i := range ops {
		if ops[i].Prefix == prefix {
			return &ops[i]
		}
	}
	return nil
}
