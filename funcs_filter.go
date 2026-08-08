package entapi

import (
	"encoding"
	"reflect"
	"sort"
	"strings"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// This file holds everything filter.tmpl needs: which fields are exposed to the
// query API, in which of the three dimensions, and — for a filterable field —
// the exact list of parameters its type earns.
//
// The operator table is deliberately absent. Which predicates exist for a field
// is a property of ent's own generator (entc/gen/func.go fieldOps, plus the sql
// storage driver's extra ops), it is already computed, and it is already
// exposed to templates as $field.Ops. Restating it here would be a second table
// that can only drift out of agreement with the where.go ent actually writes.

// queryFields returns fields carrying at least one query-dimension word.
func queryFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, f := range node.Fields {
		a := getFieldAnnotation(f)
		if a != nil && (a.Filterable || a.Searchable || a.Sortable) {
			fields = append(fields, f)
		}
	}
	return fields
}

// isFilterable reports whether a field asks for structured, per-field,
// per-operator filter parameters.
func isFilterable(f *gen.Field) bool {
	a := getFieldAnnotation(f)
	return a != nil && a.Filterable
}

// isSearchable reports whether a field takes part in the free-text disjunction.
func isSearchable(f *gen.Field) bool {
	a := getFieldAnnotation(f)
	return a != nil && a.Searchable
}

// isSortable reports whether a field may appear in the sort allow-list.
//
// The allow-list is load-bearing security rather than ergonomics: an unchecked
// sort field is an injection site, an unindexed-scan trigger and — combined
// with paging — an ordering oracle over columns the caller was never meant to
// read. A field that does not answer true here is not orderable, and no caller
// string can make it one.
func isSortable(f *gen.Field) bool {
	a := getFieldAnnotation(f)
	return a != nil && a.Sortable
}

// searchFields returns the fields the free-text term is matched against.
//
// It exists alongside isSearchable because the template has to ask a question
// isSearchable cannot answer on its own — whether the entity has any searchable
// field at all, which decides whether the free-text parameter is emitted.
func searchFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, f := range queryFields(node) {
		if isSearchable(f) {
			fields = append(fields, f)
		}
	}
	return fields
}

type parseOperator struct {
	Prefix      string
	Field       string
	SecondField string
	Kind        string
	BoolValue   bool
}

type queryParseField struct {
	Field      *gen.Field
	Name       string
	Params     []filterParam
	Operators  []parseOperator
	ParseKind  string
	Bits       int
	Enums      []string
	Conversion string
	Allowed    string
}

var opWirePrefix = map[gen.Op]string{
	gen.EQ:           "eq",
	gen.NEQ:          "ne",
	gen.GT:           "gt",
	gen.GTE:          "ge",
	gen.LT:           "lt",
	gen.LTE:          "le",
	gen.In:           "in",
	gen.NotIn:        "not_in",
	gen.Contains:     "like",
	gen.ContainsFold: "ilike",
	gen.HasPrefix:    "prefix",
	gen.HasSuffix:    "suffix",
}

var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// parseFields returns the primary key followed by the explicitly Filterable
// fields. It carries the generated semantic decisions: the field-local wire
// allow-list and the conversion expression used after lexical parsing.
func parseFields(node *gen.Type) []queryParseField {
	if node == nil {
		return nil
	}
	fields := make([]*gen.Field, 0, len(node.Fields)+1)
	if node.ID != nil {
		fields = append(fields, node.ID)
	}
	for _, f := range node.Fields {
		if isFilterable(f) {
			fields = append(fields, f)
		}
	}
	out := make([]queryParseField, 0, len(fields))
	for _, f := range fields {
		name := f.StructField()
		if f == node.ID {
			name = "ID"
		}
		pf := queryParseField{Field: f, Name: name, Enums: f.EnumValues()}
		pf.Params = filterParamsNamed(f, name)
		pf.ParseKind, pf.Bits, pf.Conversion = fieldParseConversion(f)
		pf.Operators = fieldParseOperators(f, name)
		prefixes := make([]string, len(pf.Operators))
		for i, op := range pf.Operators {
			prefixes[i] = op.Prefix
		}
		pf.Allowed = strings.Join(prefixes, ", ")
		out = append(out, pf)
	}
	return out
}

func unfilterableFields(node *gen.Type) []*gen.Field {
	if node == nil {
		return nil
	}
	var out []*gen.Field
	for _, f := range node.Fields {
		if !isFilterable(f) {
			out = append(out, f)
		}
	}
	return out
}

func fieldParseConversion(f *gen.Field) (kind string, bits int, conversion string) {
	if f == nil || f.Type == nil {
		return "", 0, ""
	}
	typ := f.Type.String()
	switch f.Type.Type {
	case field.TypeString:
		return "string", 0, typ + "(raw)"
	case field.TypeEnum:
		return "enum", 0, typ + "(raw)"
	case field.TypeBool:
		return "bool", 0, typ + "(parsed)"
	case field.TypeTime:
		return "time", 0, typ + "(parsed)"
	case field.TypeInt8:
		return "int", 8, typ + "(parsed)"
	case field.TypeInt16:
		return "int", 16, typ + "(parsed)"
	case field.TypeInt32:
		return "int", 32, typ + "(parsed)"
	case field.TypeInt:
		return "int", 0, typ + "(parsed)"
	case field.TypeInt64:
		return "int", 64, typ + "(parsed)"
	case field.TypeUint8:
		return "uint", 8, typ + "(parsed)"
	case field.TypeUint16:
		return "uint", 16, typ + "(parsed)"
	case field.TypeUint32:
		return "uint", 32, typ + "(parsed)"
	case field.TypeUint:
		return "uint", 0, typ + "(parsed)"
	case field.TypeUint64:
		return "uint", 64, typ + "(parsed)"
	case field.TypeFloat32:
		return "float", 32, typ + "(parsed)"
	case field.TypeFloat64:
		return "float", 64, typ + "(parsed)"
	case field.TypeUUID:
		return "text", 0, "parsed"
	default:
		if f.Type.RType != nil && f.Type.RType.Implements(textUnmarshalerType) {
			return "text", 0, "parsed"
		}
		return "", 0, ""
	}
}

func fieldParseOperators(f *gen.Field, fieldName string) []parseOperator {
	if f == nil {
		return nil
	}
	var (
		out        []parseOperator
		hasGTE     bool
		hasLTE     bool
		nullFields bool
	)
	for _, op := range f.Ops() {
		if op.Niladic() {
			nullFields = true
			continue
		}
		if substringOps[op] && !isSearchable(f) {
			continue
		}
		prefix, ok := opWirePrefix[op]
		if !ok {
			continue
		}
		name := fieldName + op.Name()
		if op == gen.EQ {
			name = fieldName
		}
		kind := "one"
		if op.Variadic() {
			kind = "many"
		}
		out = append(out, parseOperator{Prefix: prefix, Field: name, Kind: kind})
		hasGTE = hasGTE || op == gen.GTE
		hasLTE = hasLTE || op == gen.LTE
	}
	if nullFields {
		name := fieldName + "IsNull"
		out = append(out,
			parseOperator{Prefix: "is_null", Field: name, Kind: "null", BoolValue: true},
			parseOperator{Prefix: "not_null", Field: name, Kind: "null"},
		)
	}
	if hasGTE && hasLTE {
		out = append(out,
			parseOperator{Prefix: "from", Field: fieldName + gen.GTE.Name(), Kind: "one"},
			parseOperator{Prefix: "to", Field: fieldName + gen.LTE.Name(), Kind: "one"},
			parseOperator{Prefix: "between", Field: fieldName + gen.GTE.Name(), SecondField: fieldName + gen.LTE.Name(), Kind: "between"},
		)
	}
	return out
}

// filterParam is one parameter of a generated filter struct: one operator of
// one field, or — for the null pair — one question about two.
type filterParam struct {
	// Field is the Go struct field name, e.g. "TitleContains".
	Field string
	// Tag is the wire name used for both the form and the json tag, e.g.
	// "title_contains".
	Tag string
	// Type is the rendered Go type of the parameter, e.g. "*string".
	Type string
	// Pred is the ent predicate constructor in the entity's own package, e.g.
	// "TitleContains". Empty for Kind "null", which needs two.
	Pred string
	// Kind selects the shape of the emitted statement: "one" for a scalar
	// pointer, "many" for a variadic slice, "null" for the collapsed IsNil /
	// NotNil question.
	Kind string
}

// opTagSuffix is the wire spelling of each operator. It is a naming table, not
// an operator table: which operators exist is ent's answer, read from
// $field.Ops; this only decides what to call the parameter that carries one.
//
// EQ has no suffix, so the common case reads as the bare column name.
var opTagSuffix = map[gen.Op]string{
	gen.EQ:           "",
	gen.NEQ:          "_neq",
	gen.In:           "_in",
	gen.NotIn:        "_not_in",
	gen.GT:           "_gt",
	gen.GTE:          "_gte",
	gen.LT:           "_lt",
	gen.LTE:          "_lte",
	gen.Contains:     "_contains",
	gen.HasPrefix:    "_prefix",
	gen.HasSuffix:    "_suffix",
	gen.ContainsFold: "_icontains",
}

// substringOps is the expensive class (ADR-0005): emitted only when the
// field is also Searchable. EqualFold is exact-match SEMANTICS but sits
// here for its cost profile — LOWER(x)=LOWER(?) scans without a
// functional index, exactly like a substring match. HasPrefix joined this
// class in #83: PostgreSQL does not index-serve left-anchored LIKE 'x%' under
// the default non-C collation unless the index has an explicit pattern operator
// class (text_pattern_ops, varchar_pattern_ops or bpchar_pattern_ops), and ent
// does not create such an index by default. The old justification was
// engine-specific.
var substringOps = map[gen.Op]bool{
	gen.Contains: true, gen.ContainsFold: true,
	gen.EqualFold: true, gen.HasPrefix: true, gen.HasSuffix: true,
}

// nullTagSuffix names the one parameter the IsNil/NotNil pair collapses into.
const nullTagSuffix = "_is_null"

// filterParamsNamed returns the parameters a filterable field contributes, in
// ent's own operator order, using fieldName for generated Go identifiers.
//
// Three rules shape the result, and only three:
//
//   - Coverage starts as whatever ent derived. Emitting an operator costs
//     nothing at generation time; adding one later means editing a template,
//     regenerating and possibly breaking a consumer's URL contract.
//   - The substring class is withheld unless the field is also Searchable
//     (ADR-0005). Those operators are the ones whose cost profile matches the
//     free-text disjunction that marker already owns, and emitting one **is**
//     a permanent URL contract for a scan the annotation never asked for.
//   - IsNil and NotNil are one boolean question, so they collapse into one
//     parameter. Two would admit a request that contradicts itself, and there
//     is no honest answer to "is null AND is not null".
func filterParamsNamed(f *gen.Field, fieldName string) []filterParam {
	if f == nil {
		return nil
	}

	var (
		out      []filterParam
		nullable bool
	)
	for _, op := range f.Ops() {
		if op.Niladic() {
			// IsNil and NotNil. Recorded once, emitted after the operators
			// that take a value, so the null question reads as a footnote to
			// the field rather than as another comparison.
			nullable = true
			continue
		}

		if substringOps[op] && !isSearchable(f) {
			// ADR-0005. Existence is still ent's answer; this only decides
			// which of the operators ent derived becomes a URL parameter.
			continue
		}

		suffix, ok := opTagSuffix[op]
		if !ok {
			// An operator ent knows and this package does not. Skipping it
			// silently would hide the gap; there is no such operator today,
			// and if ent adds one the missing parameter is visible in the
			// generated file rather than mis-named in it.
			continue
		}

		name := fieldName + op.Name()
		param := filterParam{
			Field: name,
			Tag:   f.StorageKey() + suffix,
			Pred:  name,
			Kind:  "one",
			Type:  "[]" + f.Type.String(),
		}
		if op == gen.EQ {
			// The equality parameter is the field itself.
			param.Field = fieldName
		}
		if op.Variadic() {
			param.Kind = "many"
			param.Type = "[][]" + f.Type.String()
		}
		out = append(out, param)
	}

	if nullable {
		out = append(out, filterParam{
			Field: fieldName + "IsNull",
			Tag:   f.StorageKey() + nullTagSuffix,
			Type:  "[]bool",
			Kind:  "null",
		})
	}
	return out
}

// filterImports returns the import specs the filter file needs.
//
// Four are unconditional, because the artifacts that use them are emitted for
// every annotated entity — including one that marks nothing, whose filter type
// is empty and whose allow-list is empty:
//
//	entgo.io/ent/dialect/sql   the order-option signature
//	<pkg>/<entity>             OrderOption, the By<Field> builders, predicates
//	<pkg>/predicate            the return type of Predicates()
//
// The rest follow from the types of the filterable fields, exactly as
// dtoImports follows from the rendered DTO fields. Both exist because
// generation must not depend on goimports inventing an import block: formatFile
// aborts when the formatter fails, so a template that relies on the repair has
// no fallback.
func filterImports(node *gen.Type) []string {
	if node == nil {
		return nil
	}

	specs := map[string]bool{
		`"entgo.io/ent/dialect/sql"`: true,
		`"fmt"`:                      true,
		`"net/url"`:                  true,
		`"sort"`:                     true,
		`"strings"`:                  true,
	}
	if node.Config != nil {
		specs[quoteImport("", node.Config.Package+"/"+node.Package())] = true
		specs[quoteImport("", node.Config.Package+"/predicate")] = true
	}
	for _, pf := range parseFields(node) {
		if spec := fieldImportSpec(node, pf.Field); spec != "" {
			specs[spec] = true
		}
		switch pf.ParseKind {
		case "text":
			specs[`"encoding"`] = true
		case "bool", "int", "uint", "float":
			specs[`"strconv"`] = true
		}
	}

	out := make([]string, 0, len(specs))
	for spec := range specs {
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}
