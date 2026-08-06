package entdomain

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
)

// This file is the drift detector for the published annotation surface, in the
// same spirit as template_funcs_consistency_test.go is for the function
// registry. #17 existed because roughly twenty exported annotation fields were
// accepted, stored and ignored, with no way for a caller to tell which ones did
// anything. Fixing that once is worth little: the surface rotted in the first
// place because nothing failed when it did.
//
// The assertion is that every exported annotation knob is either consumed by
// generation or carries a written, issue-referenced pending status. Both
// halves are checked, in both directions.
//
// # How reachability is decided
//
// Not by a name list, and not by grepping. A knob is "reachable" iff toggling
// it changes what at least one *registered* template function returns.
//
// That is the right bar because of a property proved elsewhere:
// TestTemplateInvocationsAreRegistered requires templateFuncs() and the set of
// functions the embedded templates actually invoke to be the same set, derived
// from the parsed template trees. So "some registered function observes this
// knob" composes with "every registered function is invoked by a template" to
// give "some template observes this knob" — which is what a consumer means by
// asking whether an annotation does anything.
//
// The composition is why this test does not render templates itself. It also
// means the bar cannot be met by a helper that reads a knob but that no
// template calls: that is precisely the shape #17's verdict found for
// queryFields, and it would still fail here, because an unregistered helper is
// never probed.
//
// # How the knob list is derived
//
// By reflection over the annotation types, not by enumeration. A new exported
// field on DomainField, FieldMetadata, DomainEdge or DomainConfig enters this
// test the moment it is declared, and fails it until it is either wired to
// generation or written down as pending. That is the anti-rot property; a
// hardcoded list would rot exactly the way the surface did.

// pendingKnobs records the knobs that generation does not consume today, each
// with the reason it is still published and the issue that tracks it.
//
// An entry is a claim with a deadline, not a permanent exemption: the test
// below fails when an entry becomes reachable (the entry is now stale and must
// be deleted) just as loudly as when a knob is unreachable and unlisted. Every
// reason must name an issue, so "documented as pending" cannot degrade into
// "documented as permanent".
var pendingKnobs = map[string]string{
	// DomainField.Searchable, .Sortable, .Filterable and FieldScope.ScopeQuery
	// are deliberately absent: #27 landed filter.tmpl, so all four are consumed
	// now. isSearchable, isSortable and isFilterable each read one marker, and
	// queryFields reads the scope; all four are registered and all four are
	// invoked by templates/filter.tmpl. Their entries were removed as part of
	// that change, which is what the "reachable but still declared pending"
	// branch below would otherwise have demanded.

	// Decided on #17 (2026-07-30): kept. annotations.go labels these RESERVED
	// for OpenAPI/Swagger spec generation, which is a stated forward contract
	// rather than an unfalsifiable promise. No issue implements spec generation
	// yet; #17 is the record of the decision to keep them until one does.
	"DomainField.Metadata":     "RESERVED for spec generation; kept as a stated forward contract per the #17 verdict",
	"FieldMetadata.Title":      "RESERVED for spec generation (#17)",
	"FieldMetadata.Format":     "RESERVED for spec generation (#17)",
	"FieldMetadata.Pattern":    "RESERVED for spec generation (#17)",
	"FieldMetadata.Minimum":    "RESERVED for spec generation (#17)",
	"FieldMetadata.Maximum":    "RESERVED for spec generation (#17)",
	"FieldMetadata.MinLength":  "RESERVED for spec generation (#17)",
	"FieldMetadata.MaxLength":  "RESERVED for spec generation (#17)",
	"FieldMetadata.Enum":       "RESERVED for spec generation (#17)",
	"FieldMetadata.ReadOnly":   "RESERVED for spec generation (#17)",
	"FieldMetadata.WriteOnly":  "RESERVED for spec generation (#17)",
	"FieldMetadata.Deprecated": "RESERVED for spec generation (#17)",
	"FieldMetadata.Tags":       "RESERVED for spec generation (#17)",

	// DomainEdge.Scopes and DomainEdge.JSONKey are deliberately absent: #25
	// registered responseEdges and edgeJSONKey, so both are consumed now. They
	// were listed here while this branch was based on the commit before #25
	// landed, and this test is what caught it on rebase.

	// NOT COVERED by the #17 verdict, which enumerated the query knobs, the
	// metadata block, EntityName and the identifier types but not these three.
	// They are recorded here so the surface is fully accounted for, and raised
	// on #17 for the owner: their disposition is an annotation-surface decision
	// and is not this test's to make.
	"DomainField.Validation":  "UNDECIDED — no reader; disposition not covered by the #17 verdict, raised there for the owner",
	"DomainField.Description": "UNDECIDED — no reader; disposition not covered by the #17 verdict, raised there for the owner",
	"DomainField.Example":     "UNDECIDED — no reader; disposition not covered by the #17 verdict, raised there for the owner",
}

// annotationKnob is one exported setting a schema author can write, together
// with the mutation that turns it on.
type annotationKnob struct {
	name  string
	apply func(*surfaceProbe)
}

// surfaceProbe is the graph the registered functions are run against: one
// entity, one annotated field in TWO ent shapes, and one annotated edge.
// Everything a knob can influence is reachable from it.
//
// The two shapes are not redundancy. A knob's effect can be conditional on the
// ent schema the annotation sits on: DomainField.Required changes nothing on a
// field ent already requires — isCreateRequired says yes either way — and shows
// up only on an Optional one, where the annotation is the only reason a value
// has to be supplied. A single mandatory field therefore reported a live knob as
// dead. The same argument as meaningfulValues, one level up: probe every shape
// the knob can be observed through, not one that happens to be the wrong one.
//
// The same probe is reused for the control and the toggled observation, so the
// knob under test is provably the only difference between the two runs.
type surfaceProbe struct {
	node *gen.Type
	// graph is the one-node graph the graph-level functions take. It exists so
	// argSets can supply *gen.Graph; no annotation knob reads it today, and the
	// soft-delete marker is not a knob on any of the four annotation types this
	// file enumerates — it is attached by entdomain.SoftDeleteMixin, not written
	// by a schema author.
	graph *gen.Graph
	// field is ent-mandatory: not Optional, no Default.
	field *gen.Field
	// optional is the same annotation on an ent-Optional field.
	optional *gen.Field
	edge     *gen.Edge

	df     DomainField
	de     DomainEdge
	config DomainConfig
}

func newSurfaceProbe() *surfaceProbe {
	p := &surfaceProbe{}

	p.field = newStringField("label", &p.df)
	p.optional = newStringField("note", &p.df)
	p.optional.Optional = true
	p.node = newTestType("Probe", p.field, p.optional)
	p.node.ID = newUUIDField("id", nil)
	p.edge = &gen.Edge{
		Name:        "owner",
		Type:        p.node,
		Annotations: gen.Annotations{"DomainEdge": &p.de},
	}
	p.node.Edges = []*gen.Edge{p.edge}
	p.node.Annotations = gen.Annotations{"DomainConfig": &p.config}
	p.graph = &gen.Graph{
		Config: &gen.Config{Package: "example.com/project/ent"},
		Nodes:  []*gen.Type{p.node},
	}

	return p
}

// reset returns the probe to the control state: every annotation present but
// every knob at its zero value. The annotations must be present, or the
// difference a knob makes would be confounded with the difference the
// annotation itself makes.
func (p *surfaceProbe) reset() {
	p.df = DomainField{}
	p.de = DomainEdge{}
	p.config = DomainConfig{}
}

// observe runs every registered template function over the probe and returns a
// digest of everything they produce.
func (p *surfaceProbe) observe(t *testing.T) string {
	t.Helper()

	funcs := templateFuncs()
	names := make([]string, 0, len(funcs))
	for name := range funcs {
		names = append(names, name)
	}
	sort.Strings(names)

	var out strings.Builder
	for _, name := range names {
		fn := reflect.ValueOf(funcs[name])
		for _, args := range p.argSets(t, name, fn.Type()) {
			results := fn.Call(args)
			fmt.Fprintf(&out, "%s(%s) = ", name, describeArgs(args))
			for _, r := range results {
				out.WriteString(renderResult(r))
				out.WriteByte('|')
			}
			out.WriteByte('\n')
		}
	}
	return out.String()
}

var (
	typeGenType    = reflect.TypeOf((*gen.Type)(nil))
	typeGenField   = reflect.TypeOf((*gen.Field)(nil))
	typeGenEdge    = reflect.TypeOf((*gen.Edge)(nil))
	typeGenGraph   = reflect.TypeOf((*gen.Graph)(nil))
	typeFieldScope = reflect.TypeOf(FieldScope(""))
)

// argSets builds every argument combination a registered function should be
// called with. An unrecognised parameter type fails the test rather than being
// skipped: a silently skipped function observes nothing, which would make a
// live knob look dead.
func (p *surfaceProbe) argSets(t *testing.T, name string, ft reflect.Type) [][]reflect.Value {
	t.Helper()

	n := ft.NumIn()
	if ft.IsVariadic() {
		// Call the variadic tail empty; no registered function gives it
		// meaning, and a knob cannot reach it.
		n--
	}

	sets := [][]reflect.Value{nil}
	for i := 0; i < n; i++ {
		var candidates []reflect.Value
		switch pt := ft.In(i); {
		case pt == typeGenType:
			candidates = []reflect.Value{reflect.ValueOf(p.node)}
		case pt == typeGenField:
			candidates = []reflect.Value{
				reflect.ValueOf(p.field),
				reflect.ValueOf(p.optional),
				reflect.ValueOf(p.node.ID),
			}
		case pt == typeGenEdge:
			candidates = []reflect.Value{reflect.ValueOf(p.edge)}
		case pt == typeGenGraph:
			candidates = []reflect.Value{reflect.ValueOf(p.graph)}
		case pt == typeFieldScope:
			for _, s := range AllFieldScopes {
				candidates = append(candidates, reflect.ValueOf(s))
			}
		case pt.Kind() == reflect.String:
			candidates = []reflect.Value{reflect.ValueOf(p.field.Name)}
		default:
			t.Fatalf("templateFuncs() entry %q takes parameter %d of unhandled type %s; "+
				"teach surfaceProbe.argSets how to supply it, or this function observes nothing "+
				"and every knob it reads will look unreachable", name, i, pt)
		}

		var grown [][]reflect.Value
		for _, base := range sets {
			for _, c := range candidates {
				next := make([]reflect.Value, len(base), len(base)+1)
				copy(next, base)
				grown = append(grown, append(next, c))
			}
		}
		sets = grown
	}
	return sets
}

func describeArgs(args []reflect.Value) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		switch v := a.Interface().(type) {
		case *gen.Type:
			parts = append(parts, "type:"+v.Name)
		case *gen.Field:
			parts = append(parts, "field:"+v.Name)
		case *gen.Graph:
			parts = append(parts, "graph")
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return strings.Join(parts, ",")
}

// renderResult projects a returned value onto something comparable across two
// runs. Fields and edges are compared by name: the probe reuses one graph, but
// naming them keeps the digest readable when a difference has to be diagnosed.
func renderResult(v reflect.Value) string {
	switch out := v.Interface().(type) {
	case []*gen.Field:
		names := make([]string, len(out))
		for i, f := range out {
			names[i] = f.Name
		}
		return "[" + strings.Join(names, " ") + "]"
	case []*gen.Edge:
		names := make([]string, len(out))
		for i, e := range out {
			names[i] = e.Name
		}
		return "[" + strings.Join(names, " ") + "]"
	case []*gen.Type:
		names := make([]string, len(out))
		for i, n := range out {
			names[i] = n.Name
		}
		return "[" + strings.Join(names, " ") + "]"
	default:
		return fmt.Sprintf("%v", out)
	}
}

// annotationKnobs derives the full surface: every exported field of every
// annotation type, plus every value of the scope vocabulary.
func annotationKnobs(t *testing.T) []annotationKnob {
	t.Helper()

	var knobs []annotationKnob

	for _, f := range exportedFields(reflect.TypeOf(DomainField{})) {
		field := f
		knobs = append(knobs, annotationKnob{
			name: "DomainField." + field.Name,
			apply: func(p *surfaceProbe) {
				reflect.ValueOf(&p.df).Elem().FieldByIndex(field.Index).Set(probeValue(t, field.Type))
			},
		})
	}

	// FieldMetadata is reached through DomainField.Metadata, so each of its
	// fields is toggled on an otherwise empty metadata block.
	for _, f := range exportedFields(reflect.TypeOf(FieldMetadata{})) {
		field := f
		knobs = append(knobs, annotationKnob{
			name: "FieldMetadata." + field.Name,
			apply: func(p *surfaceProbe) {
				md := &FieldMetadata{}
				reflect.ValueOf(md).Elem().FieldByIndex(field.Index).Set(probeValue(t, field.Type))
				p.df.Metadata = md
			},
		})
	}

	for _, f := range exportedFields(reflect.TypeOf(DomainEdge{})) {
		field := f
		knobs = append(knobs, annotationKnob{
			name: "DomainEdge." + field.Name,
			apply: func(p *surfaceProbe) {
				reflect.ValueOf(&p.de).Elem().FieldByIndex(field.Index).Set(probeValue(t, field.Type))
			},
		})
	}

	for _, f := range exportedFields(reflect.TypeOf(DomainConfig{})) {
		field := f
		knobs = append(knobs, annotationKnob{
			name: "DomainConfig." + field.Name,
			apply: func(p *surfaceProbe) {
				reflect.ValueOf(&p.config).Elem().FieldByIndex(field.Index).Set(probeValue(t, field.Type))
			},
		})
	}

	// The scope vocabulary. A scope is a knob in its own right: it is published,
	// documented and settable, and DomainField.Scopes being reachable says
	// nothing about whether any particular scope value is.
	for _, s := range AllFieldScopes {
		scope := s
		knobs = append(knobs, annotationKnob{
			name: "FieldScope." + scopeConstName(scope),
			apply: func(p *surfaceProbe) {
				p.df.Scopes = []FieldScope{scope}
			},
		})
	}

	return knobs
}

func exportedFields(t reflect.Type) []reflect.StructField {
	var out []reflect.StructField
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.IsExported() {
			out = append(out, f)
		}
	}
	return out
}

// scopeConstName maps a scope value back to its Go constant name, so a failure
// names the identifier a reader would search for.
func scopeConstName(s FieldScope) string {
	switch s {
	case ScopeCreate:
		return "ScopeCreate"
	case ScopeUpdate:
		return "ScopeUpdate"
	case ScopeQuery:
		return "ScopeQuery"
	case ScopeResponse:
		return "ScopeResponse"
	default:
		return string(s)
	}
}

// meaningfulValues returns every value of an enumerable annotation type, so a
// collection knob is probed with everything it can express rather than with one
// value that might happen to be the wrong one.
//
// This is not a refinement, it is the difference between a correct and an
// incorrect answer. Filling DomainEdge.Scopes with a single arbitrary scope
// reported it as unreachable, because the only scope any edge selector looks
// for is ScopeResponse. A knob is reachable if *any* setting of it is
// observable, so probe all of them.
func meaningfulValues(t *testing.T, ft reflect.Type) []reflect.Value {
	t.Helper()

	if ft == typeFieldScope {
		out := make([]reflect.Value, len(AllFieldScopes))
		for i, s := range AllFieldScopes {
			out[i] = reflect.ValueOf(s)
		}
		return out
	}
	return []reflect.Value{probeValue(t, ft)}
}

// probeValue produces a value that a real schema author might set, which is
// stricter than "non-zero": a nonsense value would make a live knob look dead.
// FieldScope is the case that matters — a generic string would produce
// FieldScope("probe"), which no selector matches, and DomainField.Scopes would
// be misreported as unreachable.
func probeValue(t *testing.T, ft reflect.Type) reflect.Value {
	t.Helper()

	if ft == typeFieldScope {
		return reflect.ValueOf(ScopeCreate)
	}

	switch ft.Kind() {
	case reflect.Bool:
		return reflect.ValueOf(true)
	case reflect.String:
		return reflect.ValueOf("probe").Convert(ft)
	case reflect.Int, reflect.Int64:
		return reflect.ValueOf(int64(7)).Convert(ft)
	case reflect.Float64:
		return reflect.ValueOf(7.0).Convert(ft)
	case reflect.Interface:
		return reflect.ValueOf("probe").Convert(ft)
	case reflect.Pointer:
		p := reflect.New(ft.Elem())
		p.Elem().Set(probeValue(t, ft.Elem()))
		return p
	case reflect.Slice:
		values := meaningfulValues(t, ft.Elem())
		s := reflect.MakeSlice(ft, len(values), len(values))
		for i, v := range values {
			s.Index(i).Set(v)
		}
		return s
	case reflect.Map:
		m := reflect.MakeMap(ft)
		for _, k := range meaningfulValues(t, ft.Key()) {
			m.SetMapIndex(k, probeValue(t, ft.Elem()))
		}
		return m
	case reflect.Struct:
		v := reflect.New(ft).Elem()
		for _, f := range exportedFields(ft) {
			v.FieldByIndex(f.Index).Set(probeValue(t, f.Type))
		}
		return v
	default:
		t.Fatalf("no probe value for type %s (kind %s); teach probeValue about it, "+
			"or the knob using it cannot be tested", ft, ft.Kind())
		return reflect.Value{}
	}
}

// TestEveryAnnotationKnobIsConsumedOrDeclaredPending is the assertion #17
// closes on. See the file comment for how reachability and the knob list are
// derived.
func TestEveryAnnotationKnobIsConsumedOrDeclaredPending(t *testing.T) {
	probe := newSurfaceProbe()
	knobs := annotationKnobs(t)

	if len(knobs) == 0 {
		t.Fatal("no annotation knobs were derived; this test would pass vacuously")
	}

	probe.reset()
	control := probe.observe(t)

	seen := make(map[string]bool, len(knobs))
	for _, k := range knobs {
		if seen[k.name] {
			t.Errorf("knob %q derived twice; the enumeration is ambiguous", k.name)
		}
		seen[k.name] = true

		probe.reset()
		k.apply(probe)
		reachable := probe.observe(t) != control

		reason, declared := pendingKnobs[k.name]

		switch {
		case reachable && declared:
			t.Errorf("knob %q now changes generated output, but is still listed in pendingKnobs as %q.\n"+
				"delete the pendingKnobs entry: it is a claim with a deadline, and the deadline has passed",
				k.name, reason)

		case !reachable && !declared:
			t.Errorf("knob %q is exported and settable, but toggling it changes nothing any registered "+
				"template function returns, and pendingKnobs does not mention it.\n"+
				"a caller has no way to discover that it does nothing. Wire it to generation, delete it, "+
				"or add a pendingKnobs entry naming the issue that will consume it",
				k.name)

		case !reachable && declared && !strings.Contains(reason, "#"):
			t.Errorf("knob %q is declared pending as %q, which names no issue.\n"+
				"a pending status without a tracking issue is a permanent exemption in disguise",
				k.name, reason)
		}
	}

	// The reverse direction: an entry for a knob that no longer exists is a
	// stale exemption, and a typo in a knob name would silently exempt nothing
	// while appearing to exempt something.
	var stale []string
	for name := range pendingKnobs {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("pendingKnobs has %d entry/entries naming no existing knob: %s\n"+
			"delete them, or fix the name — an entry that matches nothing exempts nothing",
			len(stale), strings.Join(stale, ", "))
	}
}
