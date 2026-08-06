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
