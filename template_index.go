package entdomain

// dtoTemplate is the request/response DTO template.
var dtoTemplate = mustLoadTemplate("dto")

// baseServiceTemplate is the base service template (hooks + direct ent CRUD + EntToResponse).
var baseServiceTemplate = mustLoadTemplate("base_service")

// baseHandlerTemplate is the base handler template (ent→response conversion).
var baseHandlerTemplate = mustLoadTemplate("base_handler")

// filterTemplate is the query-surface template: filter struct, predicates and
// the sort allow-list.
var filterTemplate = mustLoadTemplate("filter")

// wiringTemplate is the wiring template: one free function per operation,
// connecting this entity's generated artifacts to the generic runtime.
var wiringTemplate = mustLoadTemplate("wiring")

// softDeleteTemplate is the soft-delete traverser and delete-rewriting hook.
// Unlike every other template here it is rendered once per GRAPH, not once per
// type: it is a single type switch over the entities embedding
// entdomain.SoftDeleteMixin, plus the one registration function a consumer
// calls.
var softDeleteTemplate = mustLoadTemplate("softdelete")
