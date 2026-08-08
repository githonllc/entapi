package entapi

// dtoTemplate is the request/response DTO template.
var dtoTemplate = mustLoadTemplate("dto")

// filterTemplate is the query-surface template: filter struct, predicates and
// the sort allow-list.
var filterTemplate = mustLoadTemplate("filter")

// wiringTemplate is the wiring template: one free function per operation,
// connecting this entity's generated artifacts to the generic runtime.
var wiringTemplate = mustLoadTemplate("wiring")

// handlerTemplate is the per-entity three-step HTTP handler template.
var handlerTemplate = mustLoadTemplate("handler")

// errorMapTemplate is the package-level error classifier the wiring returns its
// errors through. Like softDeleteTemplate it is rendered once per GRAPH: the
// wiring files all land in one package, so one declaration serves them all —
// and that is also what makes the classification identical across operations
// rather than merely intended to be.
var errorMapTemplate = mustLoadTemplate("errors")

// httpTemplate is the graph-level APIHandler and route manifest template.
var httpTemplate = mustLoadTemplate("http")

// softDeleteTemplate is the soft-delete traverser and delete-rewriting hook.
// Unlike every other template here it is rendered once per GRAPH, not once per
// type: it is a single type switch over the entities embedding
// entapi.SoftDeleteMixin.
var softDeleteTemplate = mustLoadTemplate("softdelete")

// softDeleteConfigInitTemplate is an ent partial rather than a standalone
// output file. Its config/init/fields/* definition is executed inside
// newConfig, where it installs the helpers above on every constructed client.
var softDeleteConfigInitTemplate = mustLoadTemplate("softdelete_config_init")
