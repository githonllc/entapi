---
name: entdomain
description: Work with EntDomain — the Ent code generation extension. Use when creating Ent schemas with domain annotations, understanding generated code, debugging codegen issues, or asking about EntDomain annotation builders, field scopes, generated DTOs, the query surface, or the generated wiring functions.
---

# EntDomain — Ent Code Generation Extension

You are assisting a developer working with EntDomain, an Ent Framework extension that generates HTTP DTOs, a query surface, and one wiring function per operation from annotated schemas.

## Architecture

```
your handler / service (hand-written)
     │  calls, and may stop calling, any of:
     ▼
{entity}_wiring.go (generated, ent/)   ────→  entdomain runtime  ────→  ent.Client
     │  GetX · ListXs · CreateX · UpdateX          GetOne · ListPage
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

## Field Scopes

Scopes control which **handler-layer DTOs** include a field. They do NOT restrict service layer access.

| Scope | Constant | Affects |
|-------|----------|---------|
| Create | `ScopeCreate` | `{Entity}CreateRequest` struct |
| Update | `ScopeUpdate` | `{Entity}PatchRequest` struct |
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

- `.WithRequired(scope)` — required in that scope's DTO. **The only fluent
  method besides the scope builders that changes generated output.**

Everything below is accepted and stored but generates nothing today. See the
README's "Annotation surface" section for the issue tracking each:

- `.AsSearchable()`, `.AsFilterable()`, `.AsSortable()` — opt-in per field, and the only way
  in: no preset builder grants them. They generate `{Entity}Filter`, the `q` free-text
  disjunction and `{Entity}SortKeys` in `{entity}_filter.go`. Each also needs `ScopeQuery`
  on the same field, or generation is refused.
- `.WithDescription(desc)`, `.WithExample(val)` — no reader, disposition undecided (#17)
- `.WithTitle()`, `.WithFormat()`, `.WithPattern()`, `.WithRange()`,
  `.WithLength()`, `.WithEnum()`, `.AsReadOnly()`, `.AsWriteOnly()`,
  `.AsDeprecated()`, `.WithTags()` — RESERVED for spec generation

`.AsUniqueLookup()` and `.AsRangeLookup()` were **removed** on #17; nothing ever
generated the `FindByX` methods they promised.

## Generated Files Per Entity

For entity `Courier`, three files are generated in `ent/`:

| File | Contains |
|------|----------|
| `ent/courier_dto.go` | `CourierCreateRequest`, `CourierPatchRequest`, their `Validate()`/`Apply` pair, `CourierResponse`, `CourierSummary`, `NewCourierResponse`, `CourierQueryWithResponseEdges` |
| `ent/courier_filter.go` | `CourierFilter` with `Predicates()`, `CourierSortKeys`, `CourierOrder` |
| `ent/courier_wiring.go` | `GetCourier`, `ListCouriers`, `CreateCourier`, `UpdateCourier`, `DeleteCourier`, `DeleteBatchCouriers` — free functions, one call into the runtime each |

### `ent/courier_dto.go`

- `CourierCreateRequest` — fields with `ScopeCreate`
  - `Validate() (*ValidCourierCreateRequest, error)` — required-field validation. It
    returns the only type `Apply` is defined on, so the builder cannot be written
    without validating.
  - `Has<Field>() bool` — whether the JSON payload carried that key. A field the
    caller omitted is never written, so the schema's `Default()` applies.
- `CourierPatchRequest` — fields with `ScopeUpdate` that ent can actually set, as
  pointer types (partial update)
  - `Validate() (*ValidCourierPatchRequest, error)` — rejects an explicit `null` on
    a field the schema does not declare `Optional()`
  - `Has<Field>() bool` — absent, explicit `null` and value are three states:
    absent leaves the field alone, `null` clears it, a value sets it
- `CourierResponse` — fields with `ScopeResponse`, plus nested edge responses
- `CourierListResponse` — paginated response wrapper: `data`, `total`, `page`, `size`.
  The same four fields as `entdomain.Page`, which is what the wiring returns.
  It carried a fifth, `PageInfo`, until #6 removed the cursor surface

### `ent/courier_wiring.go`

```go
func GetCourier(ctx, db *Client, id uuid.UUID) (*CourierResponse, error)
func ListCouriers(ctx, db *Client, f *CourierFilter, r entdomain.ListRequest) (*entdomain.Page[CourierResponse], error)
func CreateCourier(ctx, db *Client, v *ValidCourierCreateRequest) (*CourierResponse, error)
func UpdateCourier(ctx, db *Client, id uuid.UUID, v *ValidCourierPatchRequest) (*CourierResponse, error)
func DeleteCourier(ctx, db *Client, id uuid.UUID) error
func DeleteBatchCouriers(ctx, db *Client, ids []uuid.UUID) (int, error)
```

**These return the response DTO, not `*ent.Courier`.** Create and update take the
**validated** request — `Apply` is defined on `Valid…Request` and nowhere else, so
skipping validation is a compile error rather than a discipline problem.

The identifier type comes from the schema; `uuid.UUID` above is Courier's, not a
constraint of the generator.

`GetCourier` and `ListCouriers` read through `CourierQueryWithResponseEdges`, the
generated eager-load plan. `ent.Client.Courier.Get` applies no plan and therefore
cannot serve a response type that declares edges.

Every one of them returns its error through `ent.ErrorMap`, generated once per
package into `ent/entdomain_errors.go`. A missing row is `entdomain.ErrNotFound`
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
func (s *CourierService) ListMine(ctx context.Context, f *ent.CourierFilter, r entdomain.ListRequest) (*entdomain.Page[ent.CourierResponse], error) {
    os, err := ent.CourierOrder(r)
    if err != nil {
        return nil, err
    }
    q := ent.CourierQueryWithResponseEdges(s.db.Courier.Query())
    ps := append(f.Predicates(), courier.OrganizationIDEQ(tenantFrom(ctx)))
    return entdomain.ListPage(ctx, q, ps, os, r, ent.NewCourierResponse)
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
    resp, err := ent.UpdateCourier(c.Request.Context(), h.db, id, v)
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

## Source Files

| File | Purpose |
|------|---------|
| `annotations.go` | Annotation types, scope constants, fluent builders |
| `types.go` | ListRequest, DefaultPageSize/MaxPageSize, Ptr/PtrOrNil/PtrNilSafe helpers |
| `errors.go` | ErrNotFound, ErrAlreadyExists, ErrValidation sentinels |
| `extension.go` | Extension configuration and generation hooks |
| `funcs.go` | Template function registry |
| `funcs_fields.go` | Field filtering (createFields, updateFields, etc.) |
| `funcs_codegen.go` | Code generation helpers |
| `query.go` | GetOne, ListPage, SaveOne — the generic runtime the wiring calls |
| `templates/dto.tmpl` | Template for DTOs (CreateRequest, PatchRequest, Response, Summary, eager-load plan) |
| `templates/filter.tmpl` | Template for the filter struct, predicates and sort allow-list |
| `templates/wiring.tmpl` | Template for the per-operation free functions |
| `templates/errors.tmpl` | Template for `ErrorMap`, one declaration per package (graph-level) |
