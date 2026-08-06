package entdomain

import (
	"fmt"

	"entgo.io/ent/entc/gen"
)

// fieldValueExpr emits the expression that reads a response-scoped field off an
// ent entity, given the name of the entity variable.
//
// The response and summary structs declare *T for an Optional field while the
// entity holds T, so a conversion is required rather than optional. PtrOrNil is
// that conversion; PtrNilSafe is its counterpart for slice- and map-shaped
// types, which are not comparable and therefore cannot be tested against their
// zero value. A Nillable field already arrives as *T and is copied straight
// across.
//
// The entdomain qualifier is literal on purpose: the emitted file lands in the
// consumer's package ent and imports this package under that name.
func fieldValueExpr(field *gen.Field, recv string) string {
	switch {
	case field.Nillable:
		return fmt.Sprintf("%s.%s", recv, field.StructField())
	case field.Optional && isComplexFieldType(field):
		return fmt.Sprintf("entdomain.PtrNilSafe(%s.%s)", recv, field.StructField())
	case field.Optional:
		return fmt.Sprintf("entdomain.PtrOrNil(%s.%s)", recv, field.StructField())
	default:
		return fmt.Sprintf("%s.%s", recv, field.StructField())
	}
}
