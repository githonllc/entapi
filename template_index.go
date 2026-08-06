package entdomain

// dtoTemplate is the request/response DTO template.
var dtoTemplate = mustLoadTemplate("dto")

// filterTemplate is the query-surface template: filter struct, predicates and
// the sort allow-list.
var filterTemplate = mustLoadTemplate("filter")

// wiringTemplate is the wiring template: one free function per operation,
// connecting this entity's generated artifacts to the generic runtime.
var wiringTemplate = mustLoadTemplate("wiring")

// errorMapTemplate is the package-level error classifier the wiring returns its
// errors through. Like softDeleteTemplate it is rendered once per GRAPH: the
// wiring files all land in one package, so one declaration serves them all —
// and that is also what makes the classification identical across operations
// rather than merely intended to be.
var errorMapTemplate = mustLoadTemplate("errors")

// softDeleteTemplate is the soft-delete traverser and delete-rewriting hook.
// Unlike every other template here it is rendered once per GRAPH, not once per
// type: it is a single type switch over the entities embedding
// entdomain.SoftDeleteMixin, plus the one registration function a consumer
// calls.
var softDeleteTemplate = mustLoadTemplate("softdelete")
