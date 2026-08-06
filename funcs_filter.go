package entdomain

import (
	"sort"

	"entgo.io/ent/entc/gen"
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

// queryFields returns the fields exposed to the query API: those whose
// annotation carries ScopeQuery.
//
// It is the outer loop of every query artifact, and the three markers below are
// the inner test. Scope answers "may this field be reached from a query at
// all", the marker answers "in which dimension" — the same split the create,
// update and response scopes already use, so that withholding ScopeQuery is a
// single, visible way to keep a column out of the query surface entirely.
func queryFields(node *gen.Type) []*gen.Field {
	var fields []*gen.Field
	for _, f := range node.Fields {
		if getDomainFieldAnnotation(f) == nil {
			continue
		}
		if hasDomainScope(f, ScopeQuery) {
			fields = append(fields, f)
		}
	}
	return fields
}

// isFilterable reports whether a field asks for structured, per-field,
// per-operator filter parameters.
func isFilterable(f *gen.Field) bool {
	a := getDomainFieldAnnotation(f)
	return a != nil && a.Filterable
}

// isSearchable reports whether a field takes part in the free-text disjunction.
func isSearchable(f *gen.Field) bool {
	a := getDomainFieldAnnotation(f)
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
	a := getDomainFieldAnnotation(f)
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
	gen.EqualFold:    "_ieq",
	gen.ContainsFold: "_icontains",
}

// substringOps is the expensive class (ADR-0005): emitted only when the
// field is also Searchable. EqualFold is exact-match SEMANTICS but sits
// here for its cost profile — LOWER(x)=LOWER(?) scans without a
// functional index, exactly like a substring match.
var substringOps = map[gen.Op]bool{
	gen.Contains: true, gen.ContainsFold: true,
	gen.EqualFold: true, gen.HasSuffix: true,
}

// nullTagSuffix names the one parameter the IsNil/NotNil pair collapses into.
const nullTagSuffix = "_is_null"

// filterParams returns the parameters a filterable field contributes, in ent's
// own operator order.
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
func filterParams(f *gen.Field) []filterParam {
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

		name := f.StructField() + op.Name()
		param := filterParam{
			Field: name,
			Tag:   f.StorageKey() + suffix,
			Pred:  name,
			Kind:  "one",
			Type:  "*" + f.Type.String(),
		}
		if op == gen.EQ {
			// The equality parameter is the field itself.
			param.Field = f.StructField()
		}
		if op.Variadic() {
			param.Kind = "many"
			param.Type = "[]" + f.Type.String()
		}
		out = append(out, param)
	}

	if nullable {
		out = append(out, filterParam{
			Field: f.StructField() + "IsNull",
			Tag:   f.StorageKey() + nullTagSuffix,
			Type:  "*bool",
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

	specs := map[string]bool{`"entgo.io/ent/dialect/sql"`: true}
	if node.Config != nil {
		specs[quoteImport("", node.Config.Package+"/"+node.Package())] = true
		specs[quoteImport("", node.Config.Package+"/predicate")] = true
	}
	for _, f := range queryFields(node) {
		if !isFilterable(f) {
			continue
		}
		if spec := fieldImportSpec(node, f); spec != "" {
			specs[spec] = true
		}
	}

	out := make([]string, 0, len(specs))
	for spec := range specs {
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}
