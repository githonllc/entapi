# EntAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entapi.svg)](https://pkg.go.dev/github.com/githonllc/entapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entapi)](https://goreportcard.com/report/github.com/githonllc/entapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An [Ent](https://entgo.io) extension. Mark an entity with `api.Resource()` and
it writes request types, response types, a query surface, one wiring function
per operation, and a stdlib HTTP route tree — all into your own `ent` package,
against a runtime that imports nothing but the standard library. Field shape
comes from Ent; annotations name only deviations.

*[中文](README_zh.md)*

```go
// schema/article.go — you write this
field.String("title").
    Annotations(api.Searchable(), api.Filterable(), api.Sortable())

func (Article) Annotations() []schema.Annotation {
    return []schema.Annotation{api.Resource()}
}
```

```go
// main.go — you get this entry point
http.ListenAndServe(":8080", ent.API(client))
```

Between those two declarations the generator wrote `ArticleCreateRequest` with
three-state presence, `ParseArticleQuery` with a typed `ArticleFilter`, a
multi-key sort allow-list, `ArticleResponse` with its eager-load plan, and error
classification, five three-step handlers, and the route manifest behind
`ent.API(client)`.

> ### Status: v0, never released
>
> `git tag` is empty — this repository has never been tagged and has never
> promised an API to anyone. The versioning policy is Go's own `v0.x`
> convention: **break freely, no deprecation window**. Removed symbols are named
> in [Migration notes](#migration-notes) at the end; there are no compatibility
> aliases.
>
> The code itself is complete. The redesign proposed in `docs/DESIGN-v2.md` —
> delete the base classes, parse-don't-validate, edges through `OrErr()`,
> marker-scan cleanup, error mapping hand-written in the runtime, offset-only
> pagination — **has all landed**. The three deviations are listed under
> [Deviations from DESIGN-v2](#deviations-from-design-v2). Known defects are in
> [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md).
>
> Read the [gotchas](#gotchas) before adopting. Several of them are silent.

---

## Contents

- [Install](#install) · [Three import paths](#three-import-paths) · [Wiring it in](#wiring-it-in)
- [The annotation model](#the-annotation-model) — Ent facts plus five deviation words
- [What gets generated](#what-gets-generated)
- [Generated HTTP](#generated-http)
- [Requests: three-state presence](#requests-three-state-presence)
- [Response, summary and edges](#response-summary-and-edges)
- [The query surface](#the-query-surface) — filtering, free text, sorting, pagination
- [Wiring and error mapping](#wiring-and-error-mapping)
- [Soft delete](#soft-delete)
- [Generation can fail, and that is the design](#generation-can-fail-and-that-is-the-design)
- [What the generator does to your directory](#what-the-generator-does-to-your-directory)
- [Field shapes](#field-shapes) · [Accepted but not consumed](#accepted-but-not-consumed)
- [Gotchas](#gotchas) · [Limits](#limits) · [Deviations from DESIGN-v2](#deviations-from-design-v2) · [Migration notes](#migration-notes)

---

## Install

```bash
go get github.com/githonllc/entapi
```

`go.mod` declares `go 1.23`. The only direct dependencies outside
`golang.org/x` are `entgo.io/ent v0.14.4` and `github.com/google/uuid v1.3.0`.

> **Implementation:** `go.mod`

## Three import paths

One module, three packages, split by *when the code runs*. The root and runtime
packages are named `entapi`; the schema package is named `api`.

| Import | Imported by | Principal symbols |
|---|---|---|
| `github.com/githonllc/entapi` | your `entc.go`; schemas that embed soft delete | `Extension`, `SoftDeleteMixin` |
| `github.com/githonllc/entapi/api` | your **schema** files | `Resource`, `Hidden`, `ReadOnly`, `Searchable`, `Filterable`, `Sortable`, `Expand` |
| `github.com/githonllc/entapi/runtime` | **generated code** and your handler / service code | `ListRequest`, `SortSpec`, `Page[R]`, `ListPage`, `GetOne`, `SaveOne`, `WriteProblem`, `FieldError`, `Route`, `WithActor`/`ActorFrom`, error sentinels and mapper, filter/pointer/soft-delete helpers |

The split is load-bearing, not cosmetic. The root package embeds eight templates
with `//go:embed` and reads all eight out of the embedded filesystem **at package
init**, panicking if one is missing — importing the root package runs that
whether or not you generate anything, and drags in `embed`, ent's codegen
packages and `golang.org/x/tools/imports` behind it. (Parsing happens later, per
render: the loader returns the template source as a `string`.) `runtime/` imports
the standard library only, which is what lets it into your production binary
while the root package stays out.

> **Implementation:** `template_loader.go` — `//go:embed templates/*.tmpl`,
> `templateFS`, `loadTemplate`, `mustLoadTemplate` (returns `string`);
> `template_index.go` — `dtoTemplate`, `filterTemplate`, `wiringTemplate`,
> `handlerTemplate`, `errorMapTemplate`, `httpTemplate`, `softDeleteTemplate`,
> `softDeleteConfigInitTemplate` (eight package-level `var`s, all
> evaluated at init); `extension.go` — `renderDTOFile` and its siblings, where
> `template.New(…).Funcs(…).Parse(…)` actually runs; `runtime/types.go`,
> `runtime/query.go`, `runtime/errors.go`, `runtime/errors_map.go`,
> `runtime/filter.go`, `runtime/softdelete_context.go`

## Wiring it in

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/githonllc/entapi"
)

func main() {
    ext := entapi.NewExtensionWithOptions()

    if err := entc.Generate("./schema", &gen.Config{
        Target:  "../ent",
        Package: "your/module/ent",
    }, entc.Extensions(ext)); err != nil {
        log.Fatal(err)
    }
}
```

`WithEntAPIPackage` is the **only** option. It rewrites the runtime path the
generated files import, and its default is already
`github.com/githonllc/entapi/runtime`, so it matters only if you vendored a
copy. `NewExtension(cfg)` takes an `*ExtensionConfig` directly and is nil-safe.

The extension installs exactly one `gen.Hook`. `Templates()` returns only the
soft-delete `config/init/fields/*` partial; every standalone output is rendered
and written by the hook.

> **Implementation:** `extension.go` — `Extension`, `ExtensionConfig`,
> `NewExtension`, `NewExtensionWithOptions`, `Option`, `WithEntAPIPackage`,
> `defaultEntAPIPackage`, `Hooks`, `Templates`, `Annotations`, `Options`,
> `ConfigAnnotation`

## The annotation model

`api.Resource()` is the single entity switch. Without it an entity gets no
EntAPI files. `api.Resource().Except(api.OpCreate, ...)` removes selected public
operation surfaces; request DTOs and wiring functions stay available to the
service layer, except for a create family that cannot work at all.

Field membership is silent by default and derived from Ent:

| Ent/API fact | Generated effect |
|---|---|
| `Optional`, `Default`, `Nillable` | create pointer and requiredness |
| `Immutable` | absent from PATCH |
| `Sensitive` | absent from response and summary, still settable |
| `api.Hidden()` | absent from create, patch, response and query |
| `api.ReadOnly()` | absent from create and patch; response remains |
| `api.Searchable()` | free-text and substring query dimension |
| `api.Filterable()` | structured predicates derived from Ent's operators |
| `api.Sortable()` | enters the sort allow-list |

The five field words share one mergeable annotation. Separate spelling is
canonical and safe: `Annotations(api.Searchable(), api.Sortable())` preserves
both words through Ent's serialized schema loader. Builders use value receivers
and return copies.

### Migration from the scope model

There are no compatibility aliases. Migrate the old vocabulary by effect:

| Old spelling | New spelling |
|---|---|
| `DefaultField()` | no field annotation |
| `InputOnlyField()` | Ent `Sensitive()` |
| `OutputOnlyField()` | `api.ReadOnly()` |
| `CreateOnlyField()` | Ent `Immutable()` |
| `IdField()` | no annotation; Ent's ID is automatic |
| `AuditLogField()` | `api.ReadOnly()` |
| `NewDomainField()` | `api.Hidden()` |
| `DomainFieldWithScopes(...)` | spell the intended effect with Ent plus the five words |
| `ScopeCreate` / `ScopeUpdate` | derived from `Optional`, `Default`, `Nillable`, `Immutable` |
| `ScopeResponse` | derived; remove with `Hidden` or Ent `Sensitive` |
| `ScopeQuery` | one or more of `Searchable`, `Filterable`, `Sortable` |
| `WithRequired(ScopeCreate)` | no successor; required means `!Optional && !Default` |
| `AsSearchable` / `AsFilterable` / `AsSortable` | `api.Searchable()` / `api.Filterable()` / `api.Sortable()` |
| `AsReadOnly` | `api.ReadOnly()` |
| `AsWriteOnly` | Ent `Sensitive()` |
| metadata builders | no successor |
| `Edge().InResponse().As("key")` | `api.Expand().JSONKey("key")` |

`InputOnlyField()` was an HTTP-only promise. Ent `Sensitive()` also affects the
service layer and logging. That broader semantic is deliberate: the declaration
lives in the layer that owns secrecy.

### Edges

An edge is selected by its own annotation, never inferred from foreign-key
placement:

```go
func (Post) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
            Annotations(api.Expand().JSONKey("writer")),
    }
}
```

`Expand()` puts `Author *UserSummary` in `PostResponse` and `WithAuthor()` in
the generated eager-load plan; `JSONKey("writer")` overrides the response key.
Expansion is one level deep and is never inferred from foreign-key placement.

An annotation arrives at codegen either as its Go type or as a
`map[string]interface{}` after serialized schema loading, so every read goes
through one JSON normalisation.

> **Implementation:** `api/annotations.go`; `funcs_scope.go` —
> `getResourceAnnotation`, `getFieldAnnotation`, `getEdgeAnnotation`;
> `annotations_edge.go` — `responseEdgeSet`, `edgeJSONKey`

## What gets generated

Four files per entity carrying **`api.Resource()`**. An entity without that
single switch is skipped entirely and produces no EntAPI files.

| File | Declares |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`, `{E}PatchRequest`, a `Valid…` counterpart and `Apply` for each; `{E}Response`, `{E}Summary` and their constructors; `{E}QueryWithResponseEdges`; `{E}ListResponse` and `New{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter`, `Parse{E}Query`, `Predicates()`, `{E}SortKeys`, `{E}Order` |
| `{entity}_wiring.go` | `Get{E}`, `List{Es}`, `Create{E}`, `Patch{E}`, `Delete{E}`, `DeleteBatch{Es}` |
| `{entity}_handler.go` | reachable `{Op}{E}Fn` types and three-step bind → call → write handlers |

Plus three files per schema, each with its own emission condition:

| File | Emitted when | Declares |
|---|---|---|
| `entapi_errors.go` | at least one entity produced wiring | `ErrorMap` |
| `entapi_http.go` | at least one entity carries `api.Resource()` | `APIHandler`, `API(client)`, `ServeHTTP`, `Mount` and the unexported route manifest |
| `entapi_softdelete.go` | at least one entity embeds `SoftDeleteMixin` | the unexported query traverser and delete hook |

The soft-delete condition is independent of `api.Resource()`: an entity that is
not an HTTP resource still enters the traverser's type switch if it embeds the
mixin. The extension also supplies a `config/init/fields/*` partial that
extends Ent's own `client.go`: for each such entity, `newConfig` initializes its
hook and interceptor slices. This partial creates no standalone output file and
renders no bytes for a graph without the mixin.

Output lands in **your** `ent` package (`gen.Config.Target`), so it reads as
`ent.CreateArticle`, `ent.ArticleFilter`, `ent.ErrorMap`. That is also why an
entity name can collide with one — see
[reserved names](#generation-can-fail-and-that-is-the-design).

> **Implementation:** `extension.go` — `generatePerTypeFiles`, `perTypeFileName`,
> `renderDTOFile`, `renderFilterFile`, `renderWiringFile`, `renderHandlerFile`,
> `renderErrorMapFile`, `renderHTTPFile`, `renderSoftDeleteFile`, `pendingFile`;
> `cleanup.go` — `errorMapFileName`, `httpFileName`, `softDeleteFileName`;
> `funcs_scope.go` — `isResource`;
> `funcs_softdelete.go` — `softDeleteTypes`; authoritative symbol list:
> `schema_conflicts.go` — `derivedEntityDecls`

## Generated HTTP

`ent.API(client)` returns `*ent.APIHandler`, which is also an `http.Handler`.
Use it directly, mount its routes into a consumer mux, or compose it with
ordinary stdlib middleware:

```go
api := ent.API(client)
api.Mount(mux)
mux.Handle("/v1/", http.StripPrefix("/v1", api))
```

Each non-Excepted Resource gets exactly these Go 1.22 patterns:

| Pattern | Result |
|---|---|
| `GET /articles` | bare `{"data","total","page","size"}` page, 200 |
| `POST /articles` | bare resource, 201; no `Location` header |
| `GET /articles/{id}` | bare resource, 200 |
| `PATCH /articles/{id}` | bare resource, 200 |
| `DELETE /articles/{id}` | empty body, 204 |

Errors are RFC 9457 `application/problem+json`; `WriteProblem` emits
`type: "about:blank"`, title, status and detail, plus `field` when the chain
contains `*FieldError`. Bind failures are 400 except generated `Validate`
failures (422), unsupported media types are 415, oversized bodies are 413, and
middle-step sentinels map to 404/409/400. Unclassified errors are 500. Save-time
Ent `ValidationError` classification is deliberately deferred to #74 and is
therefore still a 500 in this slice.

POST and PATCH accept only `application/json`; media-type parameters are
allowed. Their body is capped at **1 MiB before reading, with no configuration
knob**. Unknown keys are compared against the generated create/patch tag data,
so an immutable PATCH key is rejected by name rather than silently discarded.

`WithActor` and `ActorFrom` carry authentication state through middleware.
`Route` is the stdlib-only manifest row used internally by `Mount`; exporting a
route accessor and the `With(...)` function replacements belongs to #75.

Router-level unmatched paths and methods remain the stdlib mux's plain-text
404/405 responses (including `Allow` on 405), not problem+json. This residue is
intentional: installing catch-alls would make mounting into a consumer mux
behave differently from serving the generated tree directly.

## Requests: three-state presence

A PATCH body has to separate three things a plain struct cannot:

| Payload | Means | `HasNickname()` | `Nickname` |
|---|---|---|---|
| `{}` | leave it alone | `false` | `nil` |
| `{"nickname": null}` | clear it | `true` | `nil` |
| `{"nickname": "sam"}` | set it | `true` | `&"sam"` |

Fields stay `*T`. Presence lives in a `present map[string]bool` beside the
struct, filled by the generated `UnmarshalJSON` from the raw payload's key set.
`Apply` reads exactly that:

```go
if r.HasNickname() {
    if r.Nickname == nil { b.ClearNickname() } else { b.SetNickname(*r.Nickname) }
}
```

A **create** request cannot express "clear", so an explicit `null` there is
recorded as *absent* — the field goes unwritten and your schema's `Default()`
applies. That is also what makes an explicit null on a required create field a
"required" error rather than a nil dereference.

Requiredness is checked **by presence, not by the zero value** — `0` and `false`
are values. (Strings are the exception: `== ""` says the same thing.)

### Keys match strictly

`encoding/json` fills a struct field on an exact match **or** a case-insensitive
one, while presence is recorded under the raw payload key. The two disagree for
every case variant, and the failure is silent: `PATCH {"Nickname":"sam"}`
populates the field, `HasNickname()` is still `false`, `Apply` writes nothing,
and the update reports success having changed no row.

So the generated `UnmarshalJSON` rejects — **after the raw decode and before the
struct decode** — any key that folds equal to a known tag without being exactly
equal to it:

```
unknown key "Nickname" (did you mean "nickname"?)   // wraps entapi.ErrValidation
```

A rejected request never touches your receiver. Used à la carte, the DTO still
ignores keys that match **no** tag under folding. The generated HTTP handler is
stricter: before calling this custom unmarshaller it compares raw keys with the
generated tag slice and returns a 400 naming any unknown or immutable field.
Rationale for the DTO's case rule is in
[ADR-0001](docs/adr/0001-presence-follows-encoding-json-key-matching.md).

### Validation is not optional

`Validate()` returns a *different type*, and `Apply` exists only on that type:

```go
valid, err := req.Validate()          // *ValidArticleCreateRequest
if err != nil { return err }          // wraps entapi.ErrValidation
art, err := ent.CreateArticle(ctx, client, valid)
```

The only field of `Valid{E}CreateRequest` is the unexported `r
*{E}CreateRequest`, so nothing outside the package can construct one except
`Validate()`. There is no exported function that applies an unvalidated
request — skipping validation is a compile error.

> **Implementation:** `funcs_presence.go` — `isCreatePointer`,
> `isCreateRequired`, `isPatchClearable`; `funcs_fields.go` — `createFields`,
> `patchFields` (intersected with `node.MutableFields()`); `templates/dto.tmpl`;
> generated example: `internal/fixtures/basic/basicent/widget_dto.go` —
> `WidgetPatchRequest.present`, `UnmarshalJSON`, `widgetPatchRequestTags`,
> `ValidWidgetCreateRequest`, `Validate`, `Apply`

## Response, summary and edges

`New{E}Response` returns an error, `New{E}Summary` cannot. The difference is
edges:

- Edge state is read through ent's `<Edge>OrErr()`, never a nil check —
  `loadedTypes` is unexported, so a nil pointer cannot separate *genuinely
  absent* from *not loaded*.
- **To-one edges**: `err == nil` → fill the summary; `IsNotFound(err)` → set the
  field to `nil` (**loaded-and-absent is an explicit `null`**; no edge field is
  `omitempty`); any other error → the whole function returns it.
- **To-many edges**: there is no not-found state — loaded-and-empty is an empty
  slice — so any error means the edge was not loaded, and is returned.
- **Summaries carry no edges.** That is what bounds expansion: there is no
  second level for a cycle to close through, so no runtime depth counter and no
  visited set. A three-level tree comes back one level deep.

`{E}QueryWithResponseEdges(q)` applies exactly the eager-load plan
`New{E}Response` needs. Either use it or handle that error.

A summary carries **every response-visible scalar field**, minus the edges — its
scalar half is identical to the response's. Narrowing it needs a new annotation;
nothing in the schema says which field is the brief one.

An expanded edge whose target is **not an `api.Resource()`** is a generation
error: that entity is skipped, so there is no
`<Target>Summary` to reference.

> **Implementation:** `funcs_fields.go` — `responseFields`, `responseEdges`
> (returns an error); `annotations_edge.go` — `responseEdgeSet`, `edgeJSONKey`;
> `funcs_codegen.go` — `fieldValueExpr`; `funcs_typechecks.go` —
> `isComplexFieldType`; `funcs_imports.go` — `dtoImports`; generated examples:
> `internal/fixtures/edges/edgesent/post_dto.go` — `NewPostResponse` (the
> three-branch to-one case); `internal/fixtures/edges/edgesent/user_dto.go` —
> `NewUserResponse` (to-many), `UserSummary`, `UserQueryWithResponseEdges`

### The named list type

`List{Es}` returns `*entapi.Page[{E}Response]`. `{E}ListResponse` is a
non-generic named version of the same shape, because tooling like OpenAPI /
swaggo cannot express a generic instantiation:

```go
page, err := ent.ListArticles(ctx, client, filter, req)
if err != nil { return err }

// @Success 200 {object} ent.ArticleListResponse
return c.JSON(200, ent.NewArticleListResponse(page))
```

The converter's body is a single Go type conversion (`r :=
WidgetListResponse(*p)`), and that is the point: the moment `{E}ListResponse`
and `entapi.Page` disagree on field set, types or order, that line fails to
compile in **every** generated package. A type conversion ignores struct tags,
so the tag half is pinned by a separate golden-JSON test.

> **Implementation:** `runtime/query.go` — `Page[R]`; generated example:
> `internal/fixtures/basic/basicent/widget_dto.go` — `WidgetListResponse`,
> `NewWidgetListResponse`;
> `internal/fixtures/basic/basicent/listresponse_shape_test.go`

## The query surface

The generated entry point is:

```go
filter, req, err := ent.ParseArticleQuery(r.URL.Query())
```

It parses the wire into the typed `ArticleFilter` and `entapi.ListRequest` used
by the unchanged `ListArticles` signature. Field parameter names always use the
Ent storage key, never the Go field name.

### Structured filtering — `api.Filterable()`

The wire is `field=op:value`, split on the first colon. A bare value is equality:

```text
?title=ilike:go&score=gt:30&score=le:50&status=in:draft,published
```

| Spelling | Predicate |
|---|---|
| bare value, `eq:` | equality |
| `ne:` | inequality |
| `gt:` `ge:` `lt:` `le:` | comparisons |
| `in:` `not_in:` | comma-separated membership |
| `like:` `ilike:` `prefix:` `suffix:` | string matching |
| `is_null:` `not_null:` | null predicates |
| `from:` `to:` `between:a,b` | inclusive range sugar |

Each field receives only the intersection of Ent's predicates and this wire
vocabulary. `like:`, `ilike:` and `suffix:` additionally require
`api.Searchable()`; `prefix:` does not. `_ieq` has no wire spelling. Bare `*`
and `?` remain equality literals and do not become implicit `LIKE` patterns.

Parsing follows six ordered rules: an empty bare value is ignored but empty
`eq:` is real; no colon means equality; an allowed prefix applies its operator;
a known but disallowed prefix is validation failure; an unknown prefix falls
back to whole-value equality; explicit `eq:` escapes operator-looking values.
Conversion failures name the field and value and wrap `entapi.ErrValidation`.

Repeated field parameters are separate `AND`ed predicates, including repeated
equality. Consequently scalar filter slots are slices, and `in:`/`not_in:`
slots are slices of slices. The primary key is always Filterable, uses the same
rules, and never receives searchable-only operators.

### Free text — `api.Searchable()`

`_q=value` is an `OR` across searchable fields and is `AND`ed with structured
filters. Supplying `_q` to an entity with no searchable fields is a validation
failure. Searchable-only fields contribute to `_q` but are rejected as bare
field parameters because they are not Filterable.

### Sorting — `api.Sortable()`

`_sort=created_at:desc,title,id` produces an ordered `[]entapi.SortSpec`.
`{E}Order` is the single allow-list seam: an illegal key is
`entapi.ErrValidation` and names all legal storage keys. The primary key is
always Sortable. It is appended as the final deterministic tiebreak unless it
already appears anywhere in the list; an empty sort becomes primary-key
ascending order.

### Pagination and reserved parameters

Exactly four underscore-prefixed parameters exist: `_sort`, `_page`, `_size`
and `_q`; aliases and repeated reserved parameters are rejected. `_page` and
`_size` are positive decimal integers. `_size=0` is reserved for a future
count-only mode and currently fails validation; values above 1000 are accepted
and `Limit()` clamps them. The Go-layer zero-value repair remains unchanged:
`Limit()` never returns zero and `Offset()` clamps pages below one to zero.
Unknown underscore parameters, unknown bare field names and non-Filterable bare
field names fail validation. URL keys are processed in sorted order so the
first reported error is deterministic.

Pagination is offset-only. `Page` carries `Data`, `Total`, `Page`, `Size` and
nothing else.

> **Implementation:** `funcs_filter.go` — `queryFields`, `parseFields`,
> `searchFields`, per-field operator sets and conversion expressions;
> `runtime/types.go` — `ListRequest`, `SortSpec`, `DefaultPageSize`,
> `MaxPageSize`; `runtime/urlquery.go` — lexical query parsing;
> `runtime/query.go` — `Limit`, `Offset`, `Page[R]`, `Query[Q,P,O,E]`,
> `ListPage`; `runtime/filter.go` — `AppendEach`, `AppendEachSlice`;
> `templates/filter.tmpl`; generated example:
> `internal/fixtures/query/queryent/record_filter.go` — `RecordFilter`,
> `ParseRecordQuery`, `Predicates`, `recordSortOptions`, `RecordSortKeys`,
> `RecordOrder`

## Wiring and error mapping

Free functions. No interfaces, nothing to embed. If you need different
behaviour, write your own function and stop calling the generated one.

```go
func GetArticle(ctx context.Context, db *Client, id uuid.UUID) (*ArticleResponse, error)
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entapi.ListRequest) (*entapi.Page[ArticleResponse], error)
func CreateArticle(ctx context.Context, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)
func PatchArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
func DeleteArticle(ctx context.Context, db *Client, id uuid.UUID) error
func DeleteBatchArticles(ctx context.Context, db *Client, ids []uuid.UUID) (int, error)
```

No identifier type is hardcoded anywhere — the id comes from your schema's
`$.ID.Type` and reaches the runtime as a type parameter, so an `int` primary key
needs no import at all.

Every exported wiring function returns through `ErrorMap.MapError` **exactly
once**. The file also holds unexported helpers (`{entity}Get`,
`{entity}ByID`, `{entity}Reloaded`) which exist precisely so a create or update
that re-reads through the eager-load plan does not map twice. The result is that
`errors.Is(err, entapi.ErrNotFound)` works at your handler boundary without
unwrapping ent's error types.

`ErrorMap` is emitted by the template as **one line**:

```go
var ErrorMap = entapi.NewErrorMapper(IsNotFound, IsConstraintError)
```

Both predicates are **unqualified**, so they bind to the two functions ent
generates into the **same package**. That is required: `ent.NotFoundError` and
`ent.ConstraintError` are types ent generates per consumer project and the
framework has no equivalents, which is why the runtime takes `func(error) bool`
values and never names an ent type.

**`ErrorMap` does not report uniqueness violations out of the box.**
`MapError`'s branches are: not-found → wrap `ErrNotFound`; constraint **and** you
installed a uniqueness predicate → wrap `ErrAlreadyExists`; everything else
**returned unchanged**. ent's `IsConstraintError` cannot tell `UNIQUE` from
`FOREIGN KEY`, so this is opt-in per driver:

```go
func init() {
    ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(func(err error) bool {
        var pgErr *pgconn.PgError
        return errors.As(err, &pgErr) && pgErr.Code == "23505"
    })
}
```

The cost of skipping it is a 500 where a 409 belonged; it can never produce a
wrong 409. `ErrorMap` is an ordinary package-level variable and carries no
synchronisation of its own — assign it where the client is built, before the
first request.

> **Implementation:** `templates/wiring.tmpl`, `templates/errors.tmpl`;
> `runtime/errors.go` — `ErrNotFound`, `ErrAlreadyExists`, `ErrValidation`,
> `IsNotFound`, `IsAlreadyExists`, `IsValidation`; `runtime/errors_map.go` —
> `ErrorMapper`, `NewErrorMapper`, `WithUniqueViolation`, `MapError`;
> `runtime/query.go` — `ListPage`, `GetOne`, `SaveOne`, `Saver[E]`;
> `funcs_imports.go` — `wiringImports`; generated example:
> `internal/fixtures/wiring/wiringent/article_wiring.go`

## Soft delete

Annotation-based, and enforced at ent's layer rather than in the generated
wiring:

```go
func (Doc) Mixin() []ent.Mixin { return []ent.Mixin{entapi.SoftDeleteMixin{}} }
```

```go
client := ent.NewClient(ent.Driver(drv))
```

The mixin declares an `Optional().Nillable()` `field.Time("deleted_at")` and
attaches the `DomainSoftDelete` marker; ent merges mixin annotations onto the
type, so the marker — not a column-name convention — is what says "this entity
opted in".

The generated `newConfig` installs one interceptor and one hook for every
soft-deletable entity. There is no registration call and no construction-order
dependency: `NewClient`, `Open` and `enttest.Open` all use that config. Deleted
rows disappear from **every** read, including `client.Doc.Query()` calls that
touch nothing this package generated, and `Delete` becomes an update of the
tombstone column. There is no second write, and nothing in the generated wiring
knows soft delete exists: `DeleteArticle` issues `DeleteOneID(...).Exec` and
`DeleteBatchArticles` issues `Delete().Where(IDIn(...)).Exec`, and the hook
rewrites both.

Two independent context switches let you opt out per call:

```go
entapi.WithSoftDeleted(ctx)   // reads include deleted rows
entapi.WithHardDelete(ctx)    // this delete is a real delete
```

Neither implies the other — they use two distinct unexported context key types.

The injected hook occupies index 0 and Ent applies it outermost. Hooks added
later with `client.Use` therefore run inside the soft-delete hook.

> **Implementation:** `softdelete.go` — `SoftDeleteMixin`, `SoftDeleteField`
> (`"deleted_at"`), `DomainSoftDelete`, `SoftDeleteAnnotationName`;
> `funcs_softdelete.go` — `isSoftDeletable`, `softDeleteTypes`,
> `softDeleteField`, `softDeleteImports`; `runtime/softdelete_context.go` —
> `softDeletedKey`, `hardDeleteKey`, `WithSoftDeleted`, `SoftDeletedIncluded`,
> `WithHardDelete`, `HardDeleteRequested`; `templates/softdelete.tmpl`,
> `templates/softdelete_config_init.tmpl`;
> generated example:
> `internal/fixtures/softdelete/softdeleteent/entapi_softdelete.go` —
> `softDeleteTraverser`, `softDeleteHook`; and its `client.go` `newConfig`

## Generation can fail, and that is the design

The checks run **before** `next.Generate(g)`, so a rejected schema leaves
nothing on disk — not even ent's own output. The whole graph is checked and
every problem is reported **at once** (the error reads `entapi: N schema
problem(s) prevent generation:` followed by a one-line-per-problem list). The
policy:

> An annotation that contradicts the ent schema fails generation, reporting both
> facts and the fix. Anything that can be generated correctly is generated, not
> refused.

The refusal matrix covers these contradictions:

| Refused | Why |
|---|---|
| `api.Hidden()` with any other field word | hidden has no surface on which another deviation can act |
| Ent `Sensitive()` with a query word, or with `api.ReadOnly()` | a secret cannot become a query oracle; use `Hidden` for fully inaccessible data |
| A required-no-default field blocked from create by `Hidden` or `ReadOnly`, without `Except(OpCreate)` | Ent cannot insert the row from that request |
| An empty PATCH field set without `Except(OpPatch)` | the public PATCH surface is useless |
| A field word on an edge, or `Expand` on a field | the word is attached to the wrong schema element |
| Any EntAPI word on the primary key | ID is already Filterable and Sortable; its fixed query surface is annotation-free |
| A query word while `OpList` is excepted | the query surface has been closed |
| `api.Searchable()` on a type with no `Contains` | there is no substring predicate to emit |
| `api.Filterable()` on a type with no operators | the filter group would silently do nothing |
| A Filterable field or primary key whose type cannot parse wire text | use a basic scalar, enum, `time.Time`, or a type implementing `encoding.TextUnmarshaler` |
| `api.Sortable()` on a non-comparable type | Ent generates no `ByX` order option |
| A query storage key beginning with `_` | it collides with reserved query controls |
| `api.Expand()` targeting a non-resource | the target Summary type does not exist |
| `DomainSoftDelete` naming a field the entity does not have | attaching the marker by hand is unsupported; embed `SoftDeleteMixin` instead |
| A tombstone field that is not `Optional` | ent generates no `DeletedAtIsNil` predicate and the traverser would not compile |
| A self-referential edge pair annotated on one end only | ent hands chained `edge.To(…).From(…).Annotations(…)` to the *inverse* builder, so the assoc end silently loses its annotation |
| **An entity name colliding with a symbol this extension generates** | see below |

Graph-level `API`, `APIHandler` and `ErrorMap` are reserved. An entity called `ErrorMap` makes ent emit `type ErrorMap` while
`entapi_errors.go` emits `var ErrorMap` — and Go gives types, variables and
functions **one** identifier namespace per package. The result is `redeclared in
this block`, in two files the author never wrote, with nothing naming the cause.
The same holds across entities: an entity literally named `ArticleResponse` or
`DeleteArticleFn` collides with a type generated for entity `Article`. The five
Fn names are reserved at maximum breadth even when that operation is Excepted.

The reserved-name check runs at graph level rather than inside the node loop,
because **the colliding entity does not have to be annotated** — ent generates a
type for every entity, so a bare `type ErrorMap struct{ ent.Schema }` collides
just as hard, and the node loop skips exactly those. The derived list is the
**maximal** set of names a resource can produce, so refusing a name that would
not have collided today is the
accepted cost of a refusal that is stable.

Every HTTP check skips an entity without `api.Resource()` — the same condition
the generation loop uses. Soft-delete checks and reserved names remain graph-wide.

Conversely, `Optional().Nillable()` and named `GoType`s over slices and maps *are*
generated, because correct output exists for them. See
[Field shapes](#field-shapes).

> **Implementation:** `schema_conflicts.go` — `checkGraphConflicts`,
> `nodeConflicts`, `queryConflicts`, `immutableUpdateConflict`,
> `unusableSoftDeleteField`, `asymmetricSelfEdgeConflicts`,
> `asymmetricSelfEdgeConflict`, `reservedNameConflicts`, `graphSymbolConflicts`,
> `derivedName`, `derivedEntityDecls`, `derivedEntityNames`,
> `derivedNameConflict`, `fieldHasOp`, `markerList`, `errorMapSymbol`,
> `registerSoftDeleteSymbol`, `entPlural`

## What the generator does to your directory

**A generation run is atomic as a whole.** Phase one renders every file and
formats it through `golang.org/x/tools/imports` **in memory**; phase two writes
them. Every deterministic failure — a template bug, a refused schema, source the
formatter cannot parse — lands in phase one, leaving the previous run's output
intact. A formatting failure **aborts the run and returns an error**:
`imports.Process` fails only on source it cannot parse, which is a template bug,
not a tolerable cosmetic flaw.

Each individual write is atomic too: create a temp file in the target directory,
write, `chmod 0644`, `rename` into place. The honest residue is a
millisecond-scale window if the process is hard-killed *between* two renames in
phase two. Closing it would need a directory swap, which is not atomic across
platforms.
([ADR-0003](docs/adr/0003-per-run-atomic-generation.md))

**Cleanup deletes stale files, and decides ownership by a marker.** After a
successful generation — and **only** after a successful one; a failed run
deletes nothing — the generator scans the target directory and deletes any `.go`
file meeting **both**:

1. its **first line** (the head 4096 bytes, cut at the first newline) contains
   `Code generated by entapi extension`, and
2. it was not written by this run.

This is why a schema edit no longer breaks your build: delete an entity and its
`_dto.go`/`_filter.go`/`_wiring.go`/`_handler.go` go with it, instead of lingering as a
reference to a builder ent no longer generates. For anyone upgrading past the
removal of the base classes, it also removes `_base_service.go` /
`_base_handler.go` — those names are in no list; they simply carry the same
marker.

The scan is **top-level only** (`os.ReadDir`, not `filepath.Walk`) and skips
directory entries — ent's generated subpackages (`<entity>/`, `predicate/`,
`migrate/`, …) live below the target and are never candidates. A file without
the marker is left alone and **logged** with the reason; ent's own `Code
generated by ent, DO NOT EDIT.` deliberately does not match.

> **The marker line is your escape hatch.** To take ownership of a generated
> file, delete its first line. Conversely, a file you copied out of the generated
> output and forgot to strip the header from **will be deleted**.
> ([ADR-0004](docs/adr/0004-cleanup-ownership-by-marker.md))

> **Implementation:** `extension.go` — `generatePerTypeFiles` (the two-phase
> loop), `formatFile`, `writeFormatted`, `pendingFile`; `cleanup.go` —
> `generatedMarker`, `markerScanBytes`, `removeStaleArtifacts`, `removeIfStale`,
> `hasGeneratedMarker`

## Field shapes

How ent's modifiers decide the generated request shape. This is derived from
ent, never from a second opinion — ent decides which setters exist, so any
independently derived shape shows up as a call to a method that was never
generated.

| ent schema | Create field | Required? | Clearable with `null` on patch? |
|---|---|---|---|
| `field.String("a")` | `string` | yes | no |
| `field.String("a").Default("x")` | `*string` | no | no |
| `field.String("a").Optional()` | `*string` | no | **yes** |
| `field.String("a").Optional().Nillable()` | `*string` | no | yes |
| `field.String("a").Immutable()` | `string` | yes | *absent from PATCH* |
| `field.JSON("tags", []string{}).Optional()` | `*[]string` | no | yes |

Three rules, one line each:

- A create field is a **pointer** exactly when `Optional || Default || Nillable`
  — exactly when ent can fill it without the caller.
- A create field is **required** exactly when `!Optional && !Default`.
- A patch field is **clearable** exactly when `Optional`.

`patchFields` iterates `node.MutableFields()` rather than `node.Fields`, so a
field that survives provably has a `Set<Field>`; then `Hidden` and `ReadOnly`
remove HTTP setters. `createFields` applies the same two deviations to all Ent
fields. `Sensitive` remains settable in both requests and is removed only from
responses.

On the response side, `Optional` comparable fields go through
`entapi.PtrOrNil` and `Optional` slices and maps — **including** named types
over them — go through `entapi.PtrNilSafe`, chosen by inspecting
`field.Type.RType.Kind` rather than the rendered type name.

`Apply` always emits `if r.X != nil { b.SetX(*r.X) }`, never `SetNillableX`: ent
skips the nillable setter for a type that is already nillable, so
`SetNillableTags` does not exist for an optional `field.JSON`. One uniform
branch is correct for every shape.

> **Implementation:** `funcs_presence.go` — `isCreatePointer`,
> `isCreateRequired`, `isPatchClearable`; `funcs_fields.go` — `patchFields`;
> `funcs_codegen.go` — `fieldValueExpr`; `funcs_typechecks.go` —
> `isComplexFieldType`; `runtime/types.go` — `Ptr`, `PtrOrNil`, `PtrNilSafe`;
> fixture: `internal/fixtures/fieldshapes/`

## Annotation surface

The public schema API has three mergeable annotation types and no pending
knobs. Reflection tests toggle every exported field and builder against the
registered template functions; an unreachable addition fails CI.

> **Implementation:** `api/annotations.go`; `annotation_surface_test.go` —
> `pendingKnobs`; `funcs.go` — `templateFuncs`

## Gotchas

Ordered by how quietly they hurt you.

1. **`ErrorMap` never returns `ErrAlreadyExists` until you call
   `WithUniqueViolation`.** A duplicate key passes through `MapError` unchanged
   and surfaces as a 500.
2. **`New{E}Response(nil)` returns `(nil, nil)`.** Not an error. Feed it a query
   that matched nothing and you get a pair of nils, not a not-found.
3. **`like:`, `ilike:` and `suffix:` require `api.Searchable()`.** On a
   Filterable-only string field they are known-but-disallowed operators, so the
   generated parser returns `ErrValidation`; `prefix:` remains available.
4. **Only the primary key's query dimensions are inferred.** It is always
   Filterable and Sortable; every non-ID field still requires its own query word.
5. **An all-immutable PATCH is refused** unless the resource writes
   `Except(api.OpPatch)`; the request type and wiring function still exist.
6. **An `Immutable()` field in a PATCH body is absent from the DTO.** The
   generated HTTP handler rejects it by comparing raw keys with the generated
   patch-tag data. À la carte DTO decoding still ignores unrelated keys.
7. **`entapi.IsNotFound` is not ent's `IsNotFound`.** The templates call the
   latter *unqualified* so it binds to ent's generated predicate in your package.
   Qualifying it still compiles and then silently matches nothing.
8. **A required field hidden from create blocks generation** unless create is
   excepted, made optional, or given a default.
9. **`DeleteBatch` returns a count, not an error, for ids that matched
    nothing.** That `int` is your only way to learn how many existed; an empty
    list deletes zero rows, which is ent's own reading of `IDIn` with no
    arguments rather than a guard written here.
10. **`Page.Size` is the clamped size.** The parser rejects zero and negative
    `_size`, accepts values above 1000, and `ListRequest.Limit()` clamps them.
11. **`ErrorMap` is a plain package-level variable with no synchronisation.**
    Assign it where the client is built, not while serving.
12. **A generated file you copied and edited still carries the marker** —
    cleanup will delete it. Strip the first line.

## Limits

- **Offset pagination only**, with one `COUNT` per page. It is now correct (the
  primary-key tiebreak guarantees a total order), but depth is still O(n) and
  rows can still be skipped or repeated *under concurrent writes*. There is no
  keyset alternative and no cursor type in this package.
- **Summaries are always one level deep.** There is no depth option.
- **Which scalar fields a summary carries cannot be decided from the schema**, so
  a summary carries every response-scoped field minus the edges. Narrowing it
  needs a new annotation.
- **Output shares a package with ent's own.** The generator creates no separate
  `dto` subpackage and has no option to change the directory; it establishes
  ownership of the target directory file by file, via the marker, rather than by
  owning a directory outright.
- **Scopes only control HTTP-layer struct generation.** They never restrict what
  your service layer can do with an ent entity. Anything that must be enforced
  has to be enforced where the query is built.
- **The generator package loads all five templates at package init.** Confine it
  to `entc.go` and your schema files; `runtime/` is what keeps it out of your
  binary.

## Deviations from DESIGN-v2

The header of [`docs/DESIGN-v2.md`](docs/DESIGN-v2.md) says implementation has
not started. That is **stale**: the T3 it proposed has landed in full. Three
deviations remain, all deliberate:

| Design item | Actual state |
|---|---|
| §1.6 move output into an `ent/dto` subpackage | **Not done, and superseded.** Output lands in the consumer's `ent` package; handler decoupling is achieved by the generated free functions rather than by package placement |
| §8.1 refuse generation when the directory holds files that are not ours | **Not done.** Cleanup **leaves such files in place and logs them**. It depended on §1.6's exclusive directory |
| §8.4 an `OutputPackage` config option | **Not done**, and moot without §1.6. The only option is `WithEntAPIPackage` |

T2 (the audience dimension), which the design itself deferred, is likewise
unimplemented — consistent with the design.

## Migration notes

**Breaking and behaviour change (#70):** generated `RegisterSoftDelete` has
been removed. Regenerate, then delete every `ent.RegisterSoftDelete(client)`
call; embedding `SoftDeleteMixin` now configures `NewClient`, `Open` and
`enttest.Open` automatically. The hook also moves from its former registration
position to `hooks[0]`, which Ent applies outermost, ahead of hooks added later
with `client.Use`.

**Behaviour change:** an Ent field marked `Sensitive()` no longer appears in
`{Entity}Response` or `{Entity}Summary`. This closes the response-serialization
leak where a response-scoped sensitive field was emitted; create and patch
requests are unchanged.

**Breaking and behaviour change (#72):** query parsing moved from external
form binding to generated `Parse{Entity}Query`. Two entigo differences must be
migrated together: a repeated field parameter now produces separate `AND`ed
predicates rather than an `OR`/`IN`, and a value that cannot be converted now
returns `ErrValidation` rather than being skipped. `_ieq` also has no v2 wire
spelling and has been removed from the query surface.

| Before | Query wire v2 |
|---|---|
| `q=words` | `_q=words` |
| `sort_by=created_at&order=desc` | `_sort=created_at:desc` |
| `page=2&size=50` | `_page=2&_size=50` |
| `score_gt=30` | `score=gt:30` |

All v2 wire names use the field storage key. Regenerate before changing callers;
the old and new spellings are intentionally not accepted together.

The following symbols once existed in this module and have been removed, with
**no compatibility aliases** — an alias that preserves the coupling a change
exists to remove is worse than the break.

| Removed | Use instead |
|---|---|
| generated `Update{Entity}` | generated `Patch{Entity}`; regenerate and rename call sites |
| generated `RegisterSoftDelete` | nothing at client construction — embed `SoftDeleteMixin` in the schema and regenerate |
| `Base{Entity}Service`, `Base{Entity}Handler`, `SetSelf`, generated hooks | the generated free functions (`Get{E}`, `List{Es}`, …); write your own function if you need different behaviour |
| `ExtensionConfig.GenerateBaseService`, `.GenerateBaseHandler`, `WithBaseService`, `WithBaseHandler` | nothing — the base classes are gone |
| `{Entity}EntToResponse` | `New{Entity}Response`, which returns an error rather than nil on failure |
| `Apply{Entity}CreateRequest`, `Apply{Entity}UpdateRequest` (free functions) | `Valid{Entity}…Request.Apply` |
| `Cursor`, `PageInfo`, `EncodeCursor`, `DecodeCursor`, `ListRequest.Cursor` | nothing — pagination is offset-only |
| `DomainField.Sensitive`, `AsSensitive` | mark the field `Sensitive()` in the Ent schema; it is dropped from both response tiers while remaining settable |
| `DomainField.UniqueLookup`, `.RangeLookup`, `.Validation` | `api.Filterable()` (operators derive from Ent's `$field.Ops`); generated request `Validate()` |
| `DomainConfig.EntityName` | nothing — it had no readers |
| runtime symbols living in the root package | all moved to `github.com/githonllc/entapi/runtime` |
| `ListRequest.SortBy`, `ListRequest.Order`, `ListRequest.Validate`, `ListRequest.SortKey` | `ListRequest.Sort []SortSpec`; parse with generated `Parse{Entity}Query`, validate keys in `{Entity}Order` |
| `AppendIf`, `AppendIfSlice` | `AppendEach`, `AppendEachSlice`; filter slots are slices and repeated values are `AND`ed |
| query `form` tags | generated `Parse{Entity}Query`; `ListRequest` v2 has no form tags |

## Further reading

| | |
|---|---|
| [`docs/adr/`](docs/adr/) | why the load-bearing decisions are what they are — strict key matching, the primary-key tiebreak, per-run atomicity, marker ownership, operator classification |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | the module map, anchored to source the same way this file is |
| [`docs/DESIGN-v2.md`](docs/DESIGN-v2.md) | the argument for this redesign, and which of its own first-draft claims were wrong (its status header is out of date; see above) |
| [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md) | known defects |
| [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) | how to build, test and add a fixture |
| `internal/fixtures/` | compilable evidence for every rule here: one hand-written schema per directory, with its generated output committed |
| [`README_zh.md`](README_zh.md) | 中文文档 |

## License

MIT — see [LICENSE](LICENSE).
