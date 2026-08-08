package entapi

import (
	"fmt"
	"strconv"
	"strings"
)

var queryOperators = map[string]struct{}{
	"eq": {}, "ne": {}, "gt": {}, "ge": {}, "lt": {}, "le": {},
	"in": {}, "not_in": {},
	"like": {}, "ilike": {}, "prefix": {}, "suffix": {},
	"is_null": {}, "not_null": {},
	"from": {}, "to": {}, "between": {},
}

var reservedQueryParams = map[string]struct{}{
	"_sort": {}, "_page": {}, "_size": {}, "_q": {},
}

// SplitOp splits a query value on its first colon. A value without a colon is
// returned as the operand with explicit set to false.
func SplitOp(value string) (op, operand string, explicit bool) {
	op, operand, explicit = strings.Cut(value, ":")
	if !explicit {
		return "", value, false
	}
	return op, operand, true
}

// KnownQueryOperator reports whether op belongs to the global wire vocabulary.
// Field-specific permission remains generated code.
func KnownQueryOperator(op string) bool {
	_, ok := queryOperators[op]
	return ok
}

// ParseSortSpecs parses the _sort value. Field permission is intentionally not
// checked here; generated {Entity}Order is the single allow-list seam.
func ParseSortSpecs(value string) ([]SortSpec, error) {
	if value == "" {
		return nil, nil
	}
	terms := strings.Split(value, ",")
	specs := make([]SortSpec, 0, len(terms))
	for _, term := range terms {
		key, direction, explicit := strings.Cut(term, ":")
		if key == "" {
			return nil, fmt.Errorf("%w: _sort value %q contains an empty field", ErrValidation, value)
		}
		spec := SortSpec{Key: key}
		if explicit {
			switch direction {
			case "asc":
			case "desc":
				spec.Desc = true
			default:
				return nil, fmt.Errorf("%w: _sort value %q uses unknown direction %q; legal directions: asc, desc", ErrValidation, value, direction)
			}
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// ReservedQueryValue recognizes the four framework parameters and enforces
// their single-value spelling. Repetition is not an alias for a comma list.
func ReservedQueryValue(key string, values []string) (value string, recognized bool, err error) {
	if _, recognized = reservedQueryParams[key]; !recognized {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, fmt.Errorf("%w: reserved query parameter %q must appear exactly once, got %d values", ErrValidation, key, len(values))
	}
	return values[0], true, nil
}

// ParsePageParam parses the positive, one-indexed _page and _size wire values.
// Values above MaxPageSize remain valid and are clamped by ListRequest.Limit.
func ParsePageParam(name, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s value %q is not an integer", ErrValidation, name, value)
	}
	if n <= 0 {
		if name == "_size" && n == 0 {
			return 0, fmt.Errorf("%w: _size value %q must be positive; 0 is reserved for a future count-only mode", ErrValidation, value)
		}
		return 0, fmt.Errorf("%w: %s value %q must be positive", ErrValidation, name, value)
	}
	return n, nil
}
