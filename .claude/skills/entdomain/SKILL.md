---
name: entdomain
description: Work with EntDomain — the Ent code generation extension. Use when creating Ent schemas with domain annotations, understanding generated code, debugging codegen issues, or asking about EntDomain annotation builders, field scopes, BaseService hooks, BaseHandler helpers, or generated DTOs.
---

# EntDomain — Ent Code Generation Extension

You are assisting a developer working with EntDomain, an Ent Framework extension that generates HTTP DTOs, BaseService, and BaseHandler from annotated schemas.

## Architecture

```
BaseHandler (generated, ent/)  ────→  BaseService (generated, ent/)  ────→  ent.Client
     │                                      │
     │ ToResponse, ToResponseList           │ CRUD: Create, GetByID, Update, Delete
     │ PartialUpdate (typed updater)        │ ListWithCursor, DeleteBatch
     │                                      │ Before/After hooks for all operations
     ├──────────────────────┐               ├──────────────────────┐
     │                      │               │                      │
Handler (user extends)      │          Service (user extends)      │
     │ Custom endpoints     │               │ Business logic       │
     │ Uses generated DTOs  │               │ Override hooks       │
     │                      │               │ or full methods      │
```

**Key principle**: All generated code lives in the `ent/` package. Service operates on `*ent.Entity` directly. DTOs (`CreateRequest`, `UpdateRequest`, `Response`) are in the same `ent` package — no cross-package imports needed.

## ORM-Level Interceptors (IMPORTANT)

**DO NOT manually add `OrganizationIDEQ()` or `DeletedAtIsNil()` in service code.**

The ent interceptors in `internal/database/` handle this automatically:

- **`tenant.go`**: Query interceptor injects `WHERE organization_id = X`. Mutation hook auto-sets `organization_id` on Create and scopes Update/Delete with `WHERE organization_id = X`.
- **`softdelete.go`**: Query interceptor injects `WHERE deleted_at IS NULL`.

Bypass: `ctxutil.WithSystemAccess(ctx, "reason")` for queries, `mixin.SkipSoftDelete(ctx)` for soft-delete.

## Field Scopes

Scopes control which **handler-layer DTOs** include a field. They do NOT restrict service layer access.

| Scope | Constant | Affects |
|-------|----------|---------|
| Create | `ScopeCreate` | `{Entity}CreateRequest` struct |
| Update | `ScopeUpdate` | `{Entity}UpdateRequest` struct |
| Response | `ScopeResponse` | `{Entity}Response` struct |

## Annotation Builders

| Builder | Scopes | Use For |
|---------|--------|---------|
| `DefaultField()` | create, update, response | Most business fields (name, email, status) |
| `InputOnlyField()` | create, update | Password, secrets — no response scope, so never in a response |
| `OutputOnlyField()` | response | System fields (timestamps, computed state) |
| `CreateOnlyField()` | create, response | Immutable after creation (external_id) |
| `NewDomainField()` | none | Tracked by ent but not in any HTTP struct (deleted_at, password_hash) |
| `DomainFieldWithScopes(...)` | custom | Any custom combination |

### Fluent Methods

- `.WithRequired(scope)` — required in that scope's DTO
- `.AsSearchable()`, `.AsFilterable()`, `.AsSortable()` — query capabilities
- `.AsUniqueLookup()`, `.AsRangeLookup()` — generate FindBy methods
- `.WithDescription(desc)`, `.WithExample(val)` — OpenAPI metadata

## Generated Files Per Entity

For entity `Courier`, three files are generated in `ent/`:

| File | Contains |
|------|----------|
| `ent/courier_dto.go` | `CourierCreateRequest`, `CourierUpdateRequest`, `CourierResponse`, `CourierListResponse`, `Validate()` methods |
| `ent/courier_base_service.go` | `BaseCourierService` with CRUD + Before/After hooks, `ApplyCourierCreateRequest`, `ApplyCourierUpdateRequest`, `CourierEntToResponse` |
| `ent/courier_base_handler.go` | `BaseCourierHandler` with `ToResponse`, `ToResponseList`, `PartialUpdate` |

### `ent/courier_dto.go`

- `CourierCreateRequest` — fields with `ScopeCreate`
  - `Validate()` — required field validation
- `CourierUpdateRequest` — fields with `ScopeUpdate` as pointer types (partial update)
  - `Validate()` — required field validation
- `CourierResponse` — fields with `ScopeResponse`, plus nested edge responses
- `CourierListResponse` — paginated response wrapper with `PageInfo`

### `ent/courier_base_service.go`

```go
type BaseCourierServiceHooks interface {
    BeforeCreate(ctx context.Context, req *CourierCreateRequest) error
    AfterCreate(ctx context.Context, entity *Courier) (*Courier, error)
    BeforeUpdate(ctx context.Context, id uuid.UUID, req *CourierUpdateRequest) error
    AfterUpdate(ctx context.Context, entity *Courier) (*Courier, error)
    BeforeDelete(ctx context.Context, id uuid.UUID) error
    AfterDelete(ctx context.Context, id uuid.UUID) error
}

type BaseCourierService struct {
    DB   *Client
    self BaseCourierServiceHooks
}

func (s *BaseCourierService) SetSelf(hooks BaseCourierServiceHooks)
func (s *BaseCourierService) GetByID(ctx, id) (*Courier, error)
func (s *BaseCourierService) Create(ctx, req) (*Courier, error)
func (s *BaseCourierService) Update(ctx, id, req) (*Courier, error)
func (s *BaseCourierService) Delete(ctx, id) error
func (s *BaseCourierService) DeleteBatch(ctx, ids) error
func (s *BaseCourierService) ListWithCursor(ctx, limit, cursor, order) ([]*Courier, nextCursor, error)

// Builder helpers (exported for custom service methods)
func ApplyCourierCreateRequest(builder *CourierCreate, req *CourierCreateRequest)
func ApplyCourierUpdateRequest(builder *CourierUpdateOne, req *CourierUpdateRequest)

// Entity → Response conversion
func CourierEntToResponse(entity *Courier) *CourierResponse
```

**Returns `*ent.Courier`, not a DTO**. Service retains full Ent entity capabilities.

### `ent/courier_base_handler.go`

```go
type BaseCourierHandler struct{}

func (h *BaseCourierHandler) ToResponse(entity *Courier) *CourierResponse
func (h *BaseCourierHandler) ToResponseList(entities []*Courier) []*CourierResponse
func (h *BaseCourierHandler) PartialUpdate(ctx, svc courierUpdater, id, req) (*CourierResponse, error)
```

`PartialUpdate` does Update → ToResponse in one call.

## Hook Extension Pattern

```go
type CourierService struct {
    ent.BaseCourierService  // embed generated service
    publisher event.EventPublisher
    logger    *slog.Logger
}

func NewCourierService(db *ent.Client, ...) *CourierService {
    s := &CourierService{
        BaseCourierService: ent.BaseCourierService{DB: db},
        ...
    }
    s.SetSelf(s)  // enable hook dispatch to this struct
    return s
}

// Override hooks — only implement what you need
func (s *CourierService) AfterCreate(ctx context.Context, entity *ent.Courier) (*ent.Courier, error) {
    s.publisher.Publish(event.Event{Type: "courier.created", ...})
    return entity, nil
}

func (s *CourierService) BeforeDelete(ctx context.Context, id uuid.UUID) error {
    // Check for active tasks before allowing deletion
    count, _ := s.DB.Task.Query().Where(task.CourierIDEQ(id), task.StateIn(...)).Count(ctx)
    if count > 0 { return apierror.ErrCourierHasActiveTasks }
    return nil
}
```

**Hook return value design:**
- **Before hooks** return `error`: nil = proceed, error = abort
- **After hooks** return `(*ent.Entity, error)`: default returns entity unchanged

## Handler Pattern

Handlers reference DTOs from the `ent` package directly:

```go
type Handler struct {
    service.CourierBaseHandler  // type alias → ent.BaseCourierHandler
    courierService *service.CourierService
}

func (h *Handler) Create(c *gin.Context) {
    var req ent.CourierCreateRequest
    c.ShouldBindJSON(&req)
    req.Validate()
    entity, err := h.courierService.Create(c.Request.Context(), &req)
    response.Created(c, h.ToResponse(entity))
}

func (h *Handler) Update(c *gin.Context) {
    id, _ := uuid.Parse(c.Param("id"))
    var req ent.CourierUpdateRequest
    c.ShouldBindJSON(&req)
    req.Validate()

    // One-liner: Update → ToResponse
    result, err := h.PartialUpdate(c.Request.Context(), h.courierService, id, &req)
    response.OK(c, result)
}
```

## Entity Complexity Spectrum

```
Pure CRUD                                              Complex Domain Object
──────────────────────────────────────────────────────────────────────────→

Hub            Destination      Customer       Courier         Task
│              │                │              │               │
No logic       +validation      +dedup         +password       +state machine
│              │                +search        +location       +events
│              │                │              +duty toggle    +line items
│              │                │              │               +clone
│              │                │              │               │
BaseHandler    BaseHandler      BaseHandler    BaseHandler     Custom handler
+BaseService   +BeforeCreate    +search()      +hooks          +custom service
(zero code)    (coords)         helper         +custom methods +custom methods
```

## Domain Mixins

| Mixin | Fields | Annotations |
|-------|--------|-------------|
| `DomainTimeMixin` | `created_at`, `updated_at` | OutputOnlyField |
| `DomainTenantMixin` | `organization_id` | CreateOnlyField (interceptor auto-sets) |
| `DomainSoftDeleteMixin` | `deleted_at` | NewDomainField (interceptor auto-filters) |
| `DomainMetadataMixin` | `metadata` | DefaultField (JSONB) |

## Typed Errors

```go
entdomain.ErrNotFound      // entity not found
entdomain.ErrAlreadyExists // uniqueness constraint violation
entdomain.ErrValidation    // validation failed
```

## Extension Setup (`entc.go`)

```go
ext := entdomain.NewExtensionWithOptions(
    entdomain.WithEntDomainPackage("github.com/githonllc/entdomain"),
    entdomain.WithBaseService(true),
    entdomain.WithBaseHandler(true),
)
```

## Common Issues

1. **After schema changes, always run `make generate`** to regenerate code
2. **Never edit generated files** — they are overwritten on each generation
3. **Don't manually set OrganizationID** — the tenant interceptor handles it
4. **Don't manually add DeletedAtIsNil()** — the soft-delete interceptor handles it
5. **Call `SetSelf(s)` in service constructors** — without it, hook dispatch falls back to no-op defaults
6. **DTOs are in `ent/` package** — import `ent` not `ent/domain`

## Source Files

| File | Purpose |
|------|---------|
| `annotations.go` | Annotation types, scope constants, fluent builders |
| `types.go` | PageInfo, Ptr/PtrOrNil/PtrNilSafe helpers |
| `errors.go` | ErrNotFound, ErrAlreadyExists, ErrValidation sentinels |
| `cursor.go` | Cursor, PageInfo, EncodeCursor/DecodeCursor |
| `extension.go` | Extension configuration and generation hooks |
| `funcs.go` | Template function registry |
| `funcs_fields.go` | Field filtering (createFields, updateFields, etc.) |
| `funcs_codegen.go` | Code generation helpers |
| `templates/dto.tmpl` | Template for DTOs (CreateRequest, UpdateRequest, Response) |
| `templates/base_service.tmpl` | Template for BaseService with hooks |
| `templates/base_handler.tmpl` | Template for BaseHandler with PartialUpdate |
