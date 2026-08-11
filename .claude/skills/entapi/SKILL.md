---
name: entapi
description: Work with EntAPI — the Ent code generation extension. Use when creating Ent schemas with schema-time API annotations, understanding generated code, debugging codegen issues, or asking about resources, field deviations, generated DTOs, the query surface, or generated wiring functions.
---

# EntAPI — Ent Code Generation Extension

You are assisting a developer working with EntAPI, an Ent Framework extension that generates HTTP DTOs, a query surface, and one wiring function per operation from annotated schemas.

## Architecture

```
your handler / service (hand-written)
     │  calls, and may stop calling, any of:
     ▼
{entity}_wiring.go (generated, ent/)   ────→  entapi runtime  ────→  ent.Client
     │  GetX · ListXs · CreateX · PatchX           GetOne · ListPage
     │  DeleteX · DeleteBatchXs                    SaveOne
     │  one call each; no struct, nothing to embed
     ├──────────────► {entity}_dto.go    requests · responses · eager-load plan
     └──────────────► {entity}_filter.go predicates · sort allow-list
```

**Key principle**: All generated code lives in the `ent/` package, and every
generated operation is a **free function** — there is no base type to embed, no
self-reference to install and no fixed set of override points. To change one
operation, write your own function and stop calling the generated one; the
others keep working, because nothing is registered anywhere.

> **`Base{Entity}Service` and `Base{Entity}Handler` were removed** (issue #29),
> along with `WithBaseService` / `WithBaseHandler`, `{Entity}EntToResponse`,
> `SetSelf` and the Before/After hooks. See "Migrating from `BaseService` and
> `BaseHandler`" in the library's README for the member-by-member mapping.

## ORM-Level Interceptors (IMPORTANT)

**DO NOT manually add `OrganizationIDEQ()` or `DeletedAtIsNil()` in service code.**

The ent interceptors in `internal/database/` handle this automatically:

- **`tenant.go`**: Query interceptor injects `WHERE organization_id = X`. Mutation hook auto-sets `organization_id` on Create and scopes Update/Delete with `WHERE organization_id = X`.
- **`softdelete.go`**: Query interceptor injects `WHERE deleted_at IS NULL`.

Bypass: `ctxutil.WithSystemAccess(ctx, "reason")` for queries, `mixin.SkipSoftDelete(ctx)` for soft-delete.

## Schema API

`api.Resource()` is the sole entity opt-in. Field shape comes from Ent's
`Optional`, `Default`, `Nillable`, `Immutable`, `Sensitive` and type facts.
Exactly five field words deviate: `Hidden`, `ReadOnly`, `Searchable`,
`Filterable`, `Sortable`. `api.Expand()` selects one edge into a response.

```go
func (Courier) Annotations() []schema.Annotation {
    return []schema.Annotation{api.Resource().Except(api.OpDelete)}
}

func (Courier) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").
            Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
        field.String("password_hash").Sensitive(),
        field.Time("created_at").Default(time.Now).Immutable().
            Annotations(api.ReadOnly()),
    }
}

func (Courier) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("events", Event.Type).
            Annotations(api.Expand().JSONKey("history")),
    }
}
```

Separate field words merge; `Annotations(api.Searchable(), api.Sortable())`
keeps both. `Hidden` conflicts with every other field word. Ent `Sensitive`
conflicts with query dimensions and `ReadOnly`; use `Hidden` when the field
must disappear from every HTTP surface.

## Generated Files Per Entity

For entity `Courier`, three files are generated in `ent/`:

| File | Contains |
|------|----------|
| `ent/courier_dto.go` | `CourierCreateRequest`, `CourierPatchRequest`, their `Validate()`/`Apply` pair, `CourierResponse`, `CourierSummary`, `NewCourierResponse`, `CourierQueryWithResponseEdges` |
| `ent/courier_filter.go` | `CourierFilter` with `Predicates()`, `CourierSortKeys`, `CourierOrder` |
| `ent/courier_wiring.go` | `GetCourier`, `ListCouriers`, `CreateCourier`, `PatchCourier`, `DeleteCourier`, `DeleteBatchCouriers` — free functions, one call into the runtime each |

### `ent/courier_dto.go`

- `CourierCreateRequest` — Ent fields minus `Hidden` and `ReadOnly`
  - `Validate() (*ValidCourierCreateRequest, error)` — required-field validation. It
    returns the only type `Apply` is defined on, so the builder cannot be written
    without validating.
  - `Has<Field>() bool` — whether the JSON payload carried that key. A field the
    caller omitted is never written, so the schema's `Default()` applies.
- `CourierPatchRequest` — Ent `MutableFields` minus `Hidden` and `ReadOnly`, as
  pointer types (partial update)
  - `Validate() (*ValidCourierPatchRequest, error)` — rejects an explicit `null` on
    a field the schema does not declare `Optional()`
  - `Has<Field>() bool` — absent, explicit `null` and value are three states:
    absent leaves the field alone, `null` clears it, a value sets it
  - `<Field>() (T, bool)` on the `Valid…` wrapper only — the value the payload
    carried, and whether `Apply` will `Set` it. `ok` is the third state
    `Has<Field>()` cannot answer: `ok` means a value, `!ok` with
    `Has<Field>()` means an explicit `null` that `Apply` will `Clear`, and
    `!ok` without it means absent. So a cross-field rule reads the wrapper
    instead of applying the request to a throwaway update builder and reading
    back `Mutation()`. Two field names are refused because of it — a
    patch-visible field whose Go name is `Apply`, and a patch-visible pair
    `x` / `has_x`
- `CourierResponse` — Ent fields minus `Hidden` and `Sensitive`, plus expanded edges
- `CourierListResponse` — paginated response wrapper: `data`, `total`, `page`, `size`.
  The same four fields as `entapi.Page`, which is what the wiring returns.
  It carried a fifth, `PageInfo`, until #6 removed the cursor surface

### `ent/courier_wiring.go`

```go
func GetCourier(ctx, db *Client, id uuid.UUID) (*CourierResponse, error)
func ListCouriers(ctx, db *Client, f *CourierFilter, r entapi.ListRequest) (*entapi.Page[CourierResponse], error)
func CreateCourier(ctx, db *Client, v *ValidCourierCreateRequest) (*CourierResponse, error)
func PatchCourier(ctx, db *Client, id uuid.UUID, v *ValidCourierPatchRequest) (*CourierResponse, error)
func DeleteCourier(ctx, db *Client, id uuid.UUID) error
func DeleteBatchCouriers(ctx, db *Client, ids []uuid.UUID) (int, error)
```

**These return the response DTO, not `*ent.Courier`.** Create and patch take the
**validated** request — `Apply` is defined on `Valid…Request` and nowhere else, so
skipping validation is a compile error rather than a discipline problem.

The identifier type comes from the schema; `uuid.UUID` above is Courier's, not a
constraint of the generator.

`GetCourier` and `ListCouriers` read through `CourierQueryWithResponseEdges`, the
generated eager-load plan. `ent.Client.Courier.Get` applies no plan and therefore
cannot serve a response type that declares edges.

Every one of them returns its error through `ent.ErrorMap`, generated once per
package into `ent/entapi_errors.go`. A missing row is `entapi.ErrNotFound`
whichever operation produced it, and the ent error stays in the chain.

`ErrAlreadyExists` needs one line from you, because `ent.IsConstraintError` is
true for a duplicate key and a foreign-key violation alike:

```go
func init() {
    ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(func(err error) bool { // SQLite
        return strings.Contains(err.Error(), "UNIQUE constraint failed")
    })
}
```

Forget it and duplicates come back unclassified — never mislabelled.

## Extending an operation

There is nothing to override. Write your own function and stop calling the
generated one — the generated filter, order, eager-load plan and converter are
all still available to hand to it:

```go
// what used to be BeforeCreate / AfterCreate is now ordinary control flow
func (s *CourierService) Create(ctx context.Context, req *ent.CourierCreateRequest) (*ent.CourierResponse, error) {
    if err := s.checkQuota(ctx); err != nil {          // was BeforeCreate
        return nil, err
    }
    v, err := req.Validate()
    if err != nil {
        return nil, err
    }
    resp, err := ent.CreateCourier(ctx, s.db, v)
    if err != nil {
        return nil, err
    }
    s.publisher.Publish(event.Event{Type: "courier.created"})   // was AfterCreate
    return resp, nil
}

// a list operation with a policy predicate the schema cannot express
func (s *CourierService) ListMine(ctx context.Context, f *ent.CourierFilter, r entapi.ListRequest) (*entapi.Page[ent.CourierResponse], error) {
    os, err := ent.CourierOrder(r)
    if err != nil {
        return nil, err
    }
    q := ent.CourierQueryWithResponseEdges(s.db.Courier.Query())
    ps := append(f.Predicates(), courier.OrganizationIDEQ(tenantFrom(ctx)))
    return entapi.ListPage(ctx, q, ps, os, r, ent.NewCourierResponse)
}
```

## Handler Pattern

Handlers reference DTOs from the `ent` package directly:

```go
func (h *Handler) Create(c *gin.Context) {
    var req ent.CourierCreateRequest
    if err := c.ShouldBindJSON(&req); err != nil { /* 400 */ }
    resp, err := h.courierService.Create(c.Request.Context(), &req)
    if err != nil { /* map and return */ }
    response.Created(c, resp)
}

func (h *Handler) Update(c *gin.Context) {
    id, _ := uuid.Parse(c.Param("id"))
    var req ent.CourierPatchRequest
    if err := c.ShouldBindJSON(&req); err != nil { /* 400 */ }
    v, err := req.Validate()
    if err != nil { /* 422 */ }
    resp, err := ent.PatchCourier(c.Request.Context(), h.db, id, v)
    if err != nil { /* map and return */ }
    response.OK(c, resp)
}
```

**`ShouldBindJSON` must be the decode path, not a manual struct build.** Presence
is recorded by the generated `UnmarshalJSON`, so a patch assembled in Go carries
no presence at all and every nil pointer reads as "absent".

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
call the         wrap CreateX     +search()      wrap several    hand-written
generated        with a check     helper         operations      service, calling
wiring directly                                                  wiring where it fits
```

## Domain Mixins

| Mixin | Fields | Annotations |
|-------|--------|-------------|
| `DomainTimeMixin` | `created_at`, `updated_at` | `api.ReadOnly()` |
| `DomainTenantMixin` | `organization_id` | Ent `Immutable()` (interceptor auto-sets) |
| `DomainSoftDeleteMixin` | `deleted_at` | `api.Hidden()` (interceptor auto-filters) |
| `DomainMetadataMixin` | `metadata` | no field annotation (JSONB) |

## Typed Errors

```go
entapi.ErrNotFound      // entity not found
entapi.ErrAlreadyExists // uniqueness constraint violation
entapi.ErrValidation    // validation failed
```

## Extension Setup (`entc.go`)

```go
ext := entapi.NewExtensionWithOptions(
    // The RUNTIME path, and the default (#15). Generated files import this,
    // not the generator, so nothing embeds templates in a production binary.
    entapi.WithEntAPIPackage("github.com/githonllc/entapi/runtime"),
    // info.title / info.version of the generated ent/openapi.yaml. Unset they
    // default to the ent package name plus " API" and to 0.0.0; the version is
    // never read from a git tag, because generation must not depend on
    // working-tree state (#76).
    entapi.WithOpenAPITitle("Widget Service"),
    entapi.WithOpenAPIVersion("1.4.0"),
)
// WithBaseService / WithBaseHandler were removed with the templates they
// selected (#29). Every artifact is generated unconditionally now.
```

## Common Issues

1. **After schema changes, always run `make generate`** to regenerate code
2. **Never edit generated files** — they are overwritten on each generation
3. **Don't manually set OrganizationID** — the tenant interceptor handles it
4. **Don't manually add DeletedAtIsNil()** — the soft-delete interceptor handles it
5. **Handle the error from `NewXResponse`** — a not-loaded edge is an error, not a nil field. The removed `XEntToResponse` swallowed it and returned nil
6. **DTOs are in `ent/` package** — import `ent` not `ent/domain`
7. **Old `{entity}_base_service.go` / `{entity}_base_handler.go` are deleted by the next generation run** — do not re-add them by hand
8. **`ent/openapi.yaml` is generated and committed** — it is served at `GET /openapi.yaml` from an embedded copy, so edit the schema and regenerate rather than editing the document. Deleting its first `#` marker line only removes it from cleanup's deletion candidates (it survives once it stops being generated); it does **not** stop the next generation from overwriting it. For a `servers` entry or a path prefix, keep your own document under your own name and serve it from a router you build from `Endpoints()`, skipping the generated `GET /openapi.yaml` row

## Source Files

Three packages, and which one a file belongs to is decided by when it runs (#15, #71).

**`github.com/githonllc/entapi` — generation time.** Imported by `entc.go` and
schemas embedding `SoftDeleteMixin`.

| File | Purpose |
|------|---------|
| `softdelete.go` | `SoftDeleteMixin` and the `DomainSoftDelete` marker |
| `extension.go` | Extension configuration and generation hooks |
| `funcs.go` | Template function registry |
| `funcs_fields.go` | Field filtering (createFields, updateFields, etc.) |
| `funcs_codegen.go` | Code generation helpers |

**`github.com/githonllc/entapi/api` — schema time.** `api/annotations.go`
contains the three mergeable annotation types and every public schema word. It
imports only `entgo.io/ent/schema` and stdlib.

**`github.com/githonllc/entapi/runtime` — run time.** Package name is still
`entapi`. Imported by generated code and by consumer service/handler code.
Standard library only: it embeds no template and reaches neither ent nor the
formatter.

| File | Purpose |
|------|---------|
| `runtime/types.go` | ListRequest, DefaultPageSize/MaxPageSize, Ptr/PtrOrNil/PtrNilSafe helpers |
| `runtime/errors.go` | ErrNotFound, ErrAlreadyExists, ErrValidation sentinels |
| `runtime/errors_map.go` | `ErrorMapper`, which takes its classification as function values |
| `runtime/query.go` | GetOne, ListPage, SaveOne — the generic runtime the wiring calls |
| `runtime/filter.go` | `AppendIf`/`AppendIfSlice`, used by every generated filter |
| `runtime/softdelete_context.go` | `WithSoftDeleted`/`WithHardDelete` and their predicates |
| `templates/dto.tmpl` | Template for DTOs (CreateRequest, PatchRequest, Response, Summary, eager-load plan) |
| `templates/filter.tmpl` | Template for the filter struct, predicates and sort allow-list |
| `templates/wiring.tmpl` | Template for the per-operation free functions |
| `templates/errors.tmpl` | Template for `ErrorMap`, one declaration per package (graph-level) |
