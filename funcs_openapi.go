package entapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
	entapiruntime "github.com/githonllc/entapi/runtime"
)

// This file holds everything templates/openapi.tmpl needs beyond the selectors
// the Go templates already use.
//
// Two rules govern it, and they are the reason the document is trustworthy at
// all:
//
//   - Nothing here is a second opinion. Paths and operations come from
//     resourceOps/routePath — the same source templates/http.tmpl routes from.
//     Schemas come from responseFields/responseEdges/createFields/patchFields.
//     Query parameters come from parseFields and the per-field operator sets in
//     funcs_filter.go. A field, an operation or an operator this file could
//     name on its own would be a second truth that can only drift.
//   - Nothing here serializes YAML in general. yamlQuote is the whole
//     mechanism: YAML 1.2 is a strict superset of JSON, so a JSON string
//     literal IS a YAML double-quoted scalar, and every composite this file
//     emits is a JSON flow mapping — which YAML parses as a mapping. That is
//     what keeps a YAML library out of this module.

// defaultOpenAPIVersion is info.version when the extension is not configured
// with one. It is a placeholder rather than a guess: the alternative — reading
// a git tag at generation time — would make a clean checkout stop staying clean
// across a test run, which is the invariant TestCodegenFixtures rests on.
const defaultOpenAPIVersion = "0.0.0"

// yamlQuote renders s as a YAML double-quoted scalar.
//
// Every data-derived scalar in openapi.tmpl goes through here, INCLUDING map
// keys. A storage key spelled "on", "no", "y" or "null" is a boolean or a null
// to a YAML 1.1 parser if it is left bare, and a path key contains braces; a
// quoted scalar is one of exactly one thing in every parser.
func yamlQuote(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Without this, "<", ">" and "&" come back as <-style escapes. Legal,
	// but this file lands in a pull request diff and is meant to be read.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		// Encoding a string cannot fail: invalid UTF-8 is replaced rather than
		// rejected. The branch exists so the error is not discarded silently.
		return `""`
	}
	return strings.TrimRight(buf.String(), "\n")
}

// openapiSchema renders one field's JSON Schema as a flow mapping.
//
// nullable is the caller's question, not the field's: the same ent field is
// non-nullable in a response (an Optional field is omitted, not null) and
// nullable in a patch request (an explicit null is how clearing is expressed).
// Deriving it here from f.Optional alone would state one of those two wrongly.
func openapiSchema(f *gen.Field, nullable bool) string {
	if f == nil || f.Type == nil {
		return "{}"
	}
	base, extra := openapiFieldType(f)
	if base == "" {
		// A type this generator has no honest JSON Schema for — a JSON column
		// over an arbitrary Go value. An unconstrained schema is the true
		// statement; inventing "object" would not be.
		return "{}"
	}
	typeExpr := yamlQuote(base)
	if nullable {
		typeExpr = "[" + yamlQuote(base) + ", " + yamlQuote("null") + "]"
	}
	parts := append([]string{`"type": ` + typeExpr}, extra...)
	return "{" + strings.Join(parts, ", ") + "}"
}

// openapiFieldType maps an ent field type onto a JSON Schema type plus whatever
// keywords narrow it. It reads field.Type rather than the rendered Go type, for
// the reason isComplexFieldType does: a named GoType over string is still a
// string on the wire.
func openapiFieldType(f *gen.Field) (string, []string) {
	if f.IsEnum() {
		values := f.EnumValues()
		quoted := make([]string, 0, len(values))
		for _, v := range values {
			quoted = append(quoted, yamlQuote(v))
		}
		if len(quoted) == 0 {
			return "string", nil
		}
		return "string", []string{`"enum": [` + strings.Join(quoted, ", ") + `]`}
	}
	switch f.Type.Type {
	case field.TypeBool:
		return "boolean", nil
	case field.TypeString:
		return "string", nil
	case field.TypeTime:
		return "string", []string{`"format": "date-time"`}
	case field.TypeUUID:
		return "string", []string{`"format": "uuid"`}
	case field.TypeBytes:
		// encoding/json renders []byte as base64, so the wire shape is a string.
		return "string", []string{`"contentEncoding": "base64"`}
	case field.TypeInt8, field.TypeInt16, field.TypeInt32:
		return "integer", []string{`"format": "int32"`}
	case field.TypeInt, field.TypeInt64:
		return "integer", []string{`"format": "int64"`}
	case field.TypeUint8, field.TypeUint16, field.TypeUint32:
		return "integer", []string{`"format": "int32"`, `"minimum": 0`}
	case field.TypeUint, field.TypeUint64:
		return "integer", []string{`"format": "int64"`, `"minimum": 0`}
	case field.TypeFloat32:
		return "number", []string{`"format": "float"`}
	case field.TypeFloat64:
		return "number", []string{`"format": "double"`}
	default:
		return "", nil
	}
}

// openapiRequiredCreateFields returns the create fields a caller must supply.
//
// It exists because the template needs the ANSWER to "is this list empty" as
// well as the list: OpenAPI forbids an empty `required` array, so the key has
// to disappear rather than be emitted with nothing under it.
func openapiRequiredCreateFields(node *gen.Type) []*gen.Field {
	var out []*gen.Field
	for _, f := range createFields(node) {
		if isCreateRequired(f) {
			out = append(out, f)
		}
	}
	return out
}

// openapiPathGroup is the operations of one URL path. OpenAPI keys paths, not
// operations, so the flat resourceOps list has to be folded onto its two
// possible suffixes — and folding it here rather than in the template keeps the
// document's path set literally derived from the route manifest's.
type openapiPathGroup struct {
	Suffix string
	Ops    []resourceOperation
}

// openapiPathGroups folds resourceOps onto its distinct path suffixes, in first
// appearance order, so the document's path order matches the route manifest's.
func openapiPathGroups(node *gen.Type) []openapiPathGroup {
	var out []openapiPathGroup
	index := map[string]int{}
	for _, op := range resourceOps(node) {
		i, ok := index[op.PathSuffix]
		if !ok {
			index[op.PathSuffix] = len(out)
			out = append(out, openapiPathGroup{Suffix: op.PathSuffix})
			i = len(out) - 1
		}
		out[i].Ops = append(out[i].Ops, op)
	}
	return out
}

// openapiParam is one documented query parameter: its wire name, its schema as
// a flow mapping, and the prose that carries whatever OpenAPI cannot express.
type openapiParam struct {
	Name        string
	Schema      string
	Description string
}

// openapiFilterParams documents the per-field query surface.
//
// Every filter parameter degrades to a string, which is the price of the
// op-in-value wire format (ADR-0007): "gt:5" is not an integer. The pattern and
// the description are therefore the only place the contract survives, and both
// are derived — the operator list comes from parseFields, which intersects
// ent's own $field.Ops with the wire vocabulary and applies the Searchable gate.
// Nothing here may name an operator.
func openapiFilterParams(node *gen.Type) []openapiParam {
	fields := parseFields(node)
	out := make([]openapiParam, 0, len(fields))
	for _, pf := range fields {
		prefixes := make([]string, 0, len(pf.Operators))
		for _, op := range pf.Operators {
			prefixes = append(prefixes, op.Prefix)
		}
		schema := `{"type": "string"}`
		if len(prefixes) > 0 {
			pattern := "^(?:(" + strings.Join(prefixes, "|") + "):)?"
			schema = `{"type": "string", "pattern": ` + yamlQuote(pattern) + `}`
		}
		out = append(out, openapiParam{
			Name:        pf.Field.StorageKey(),
			Schema:      schema,
			Description: openapiFilterDescription(pf.Field, prefixes),
		})
	}
	return out
}

// openapiFilterDescription restates the six parsing rules a caller cannot see
// in a `type: string` schema. Each sentence corresponds to one branch of the
// generated parser in templates/filter.tmpl.
func openapiFilterDescription(f *gen.Field, prefixes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Filter on %s. ", f.StorageKey())
	b.WriteString("A bare value means eq. ")
	if len(prefixes) > 0 {
		fmt.Fprintf(&b, "Any other comparison is written as \"<operator>:<value>\"; this field accepts %s. ",
			strings.Join(prefixes, ", "))
	}
	b.WriteString("A prefix outside the global operator vocabulary is taken literally, " +
		"so \"eq:\" is the escape for a value that begins with one. ")
	b.WriteString("An operator in that vocabulary but outside this field's set is rejected with 400. ")
	b.WriteString("Repeating the parameter is legal and ANDs the resulting predicates. ")
	b.WriteString("\"in:\" and \"between:\" split their value on commas, so a value containing a comma " +
		"cannot be expressed through them; \"is_null:\" and \"not_null:\" take no value at all.")
	return b.String()
}

// openapiReservedParams documents the four framework parameters.
//
// _q is emitted only for an entity with a Searchable field, because that is
// exactly when the generated parser accepts it — anywhere else it is a 400.
// The bounds come from the runtime constants rather than from literals here,
// so the document cannot claim a page size the runtime does not apply.
func openapiReservedParams(node *gen.Type) []openapiParam {
	out := []openapiParam{
		{
			Name:   "_page",
			Schema: `{"type": "integer", "minimum": 1}`,
			Description: "One-indexed page number. It must appear at most once; " +
				"zero and negative values are rejected with 400.",
		},
		{
			Name: "_size",
			// No `maximum`: a larger value is ACCEPTED and clamped, not
			// rejected, so a maximum keyword would document a 400 that the
			// generated handler never returns.
			Schema: `{"type": "integer", "minimum": 1}`,
			Description: fmt.Sprintf("Page size. It must appear at most once; zero and negative values are "+
				"rejected with 400. Omitted it is %d, and a value above %d is clamped to %d rather than refused.",
				entapiruntime.DefaultPageSize, entapiruntime.MaxPageSize, entapiruntime.MaxPageSize),
		},
		{
			Name:   "_sort",
			Schema: `{"type": "string"}`,
			Description: fmt.Sprintf("Comma-separated sort keys, each optionally suffixed with \":asc\" or "+
				"\":desc\" (ascending by default). Legal keys: %s. The primary key is appended as a tiebreak "+
				"when it is absent from the list. Any other key is rejected with 400.",
				strings.Join(openapiSortKeys(node), ", ")),
		},
	}
	if len(searchFields(node)) > 0 {
		names := make([]string, 0, len(searchFields(node)))
		for _, f := range searchFields(node) {
			names = append(names, f.StorageKey())
		}
		out = append(out, openapiParam{
			Name:   "_q",
			Schema: `{"type": "string"}`,
			Description: fmt.Sprintf("Free-text term matched as a substring against any of %s.",
				strings.Join(names, ", ")),
		})
	}
	return out
}

// openapiSortKeys is the sort allow-list, derived exactly as templates/filter.tmpl
// derives {Entity}SortKeys: the primary key, which is always sortable, plus the
// fields carrying api.Sortable().
func openapiSortKeys(node *gen.Type) []string {
	if node == nil {
		return nil
	}
	var out []string
	if node.ID != nil {
		out = append(out, node.ID.StorageKey())
	}
	for _, f := range queryFields(node) {
		if isSortable(f) {
			out = append(out, f.StorageKey())
		}
	}
	return out
}

// openapiStatus is one problem+json response the document declares.
type openapiStatus struct {
	// Code is the HTTP status as a string, which is how OpenAPI keys it.
	Code string
	// Name is the components/responses key, e.g. "Problem404".
	Name string
	// Title is http.StatusText, i.e. the same string the generated handler
	// passes to entapi.WriteProblem as the problem title.
	Title string
}

// errorStatusesByOp is the statuses each generated handler can actually write.
// It used to be read off templates/handler.tmpl's literal http.Status
// constants; since #103 folded the status ladder into entapi.Status, it is
// derived from that function's behaviour together with the template's call
// sites — TestErrorStatusesByOpMatchesHandlerTemplate finds each
// entapi.Status/entapi.BindJSON call in a section and probes the real
// runtime.Status with every sentinel to build the expected set. Which
// operations bind, and what onValidation each passes, stay template
// properties, which is why this table cannot simply move to the runtime.
//
// It is one table so that openapiProblemStatuses below is its union rather
// than a second list.
var errorStatusesByOp = map[api.Op][]int{
	api.OpList:   {http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	api.OpGet:    {http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	api.OpDelete: {http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	api.OpCreate: {
		http.StatusBadRequest, http.StatusNotFound, http.StatusConflict,
		http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity, http.StatusInternalServerError,
	},
	api.OpPatch: {
		http.StatusBadRequest, http.StatusNotFound, http.StatusConflict,
		http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity, http.StatusInternalServerError,
	},
}

// openapiErrorStatuses returns the problem+json responses one operation declares.
func openapiErrorStatuses(op resourceOperation) []openapiStatus {
	codes := errorStatusesByOp[op.Name]
	out := make([]openapiStatus, 0, len(codes))
	for _, code := range codes {
		out = append(out, newOpenAPIStatus(code))
	}
	return out
}

// openapiProblemStatuses is the union of everything openapiErrorStatuses can
// return, ascending. The components/responses section is generated from it, so
// a status an operation names always has an entry to point at.
func openapiProblemStatuses() []openapiStatus {
	seen := map[int]bool{}
	var codes []int
	for _, list := range errorStatusesByOp {
		for _, code := range list {
			if !seen[code] {
				seen[code] = true
				codes = append(codes, code)
			}
		}
	}
	sort.Ints(codes)
	out := make([]openapiStatus, 0, len(codes))
	for _, code := range codes {
		out = append(out, newOpenAPIStatus(code))
	}
	return out
}

func newOpenAPIStatus(code int) openapiStatus {
	text := strconv.Itoa(code)
	return openapiStatus{Code: text, Name: "Problem" + text, Title: http.StatusText(code)}
}
