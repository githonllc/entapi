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
classification, five three-step handlers, and the endpoint manifest behind
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
- [Gotchas](#gotchas) · [Limits](#limits) · [Deviations from DESIGN-v2](#deviations-from-design-v2) · [Deviations from DESIGN-v3](#deviations-from-design-v3) · [Migration notes](#migration-notes)

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
| `github.com/githonllc/entapi/runtime` | **generated code** and your handler / service code | `ListRequest`, `SortSpec`, `Page[R]`, `ListPage`, `GetOne`, `SaveOne`, `BindJSON`, `Status`, `WriteJSON`, `WriteProblem`, `FieldError`, `Endpoint`/`Op`, `WithActor`/`ActorFrom`, error sentinels and mapper, filter/pointer/soft-delete helpers |

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

There are four options. `WithEntAPIPackage` rewrites the runtime path the
generated files import, and its default is already
`github.com/githonllc/entapi/runtime`, so it matters only if you vendored a
copy. `WithStrictQueryOperators()` makes an unrecognised operator prefix a
validation failure; it is off by default so bare RFC-3339 timestamps keep
working as whole-value equality literals. `WithOpenAPITitle` and
`WithOpenAPIVersion` set `info.title` and
`info.version` of the generated `openapi.yaml`; unset they default to the ent
package name plus `" API"` and to `0.0.0`. The version is deliberately NOT read
from a git tag — generation must not depend on working-tree state, or a clean
checkout stops staying clean across a test run. `NewExtension(cfg)` takes an
`*ExtensionConfig` directly and is nil-safe.

The extension installs exactly one `gen.Hook`. `Templates()` returns only the
soft-delete `config/init/fields/*` partial; every standalone output is rendered
and written by the hook.

> **Implementation:** `extension.go` — `Extension`, `ExtensionConfig`,
> `NewExtension`, `NewExtensionWithOptions`, `Option`, `WithEntAPIPackage`,
> `WithStrictQueryOperators`, `WithOpenAPITitle`, `WithOpenAPIVersion`,
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
An edge is silent by default; `api.Expand()` includes it. On a self-referential
pair, the two ends must agree on annotation presence: put `api.Expand()` on the
included end and `api.EdgeAnnotation{}` on the excluded end. The zero-value
annotation marks that end as deliberately considered and left out, making a
one-way self-referential expansion expressible without a second edge word.

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

Plus five files per schema, each with its own emission condition:

| File | Emitted when | Declares |
|---|---|---|
| `entapi_errors.go` | at least one entity produced wiring | `ErrorMap` |
| `entapi_http.go` | at least one entity carries `api.Resource()` | `APIOption`, `APIHandler`, `API(client)`, `With`, `Endpoints`, one named `…Endpoint()` accessor per reachable operation (the wiring function's name plus `Endpoint`: `GetArticleEndpoint()`, `ListArticlesEndpoint()`, …), `OpenAPIEndpoint()`, `ServeHTTP`, `Mount` and the endpoint manifest |
| `openapi.yaml` | at least one entity produced wiring | the OpenAPI 3.1 document describing every generated endpoint |
| `entapi_openapi.go` | at least one entity produced wiring | the `//go:embed` of that document and the unexported handler serving it |
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
> `renderErrorMapFile`, `renderHTTPFile`, `renderOpenAPIFile`,
> `renderOpenAPIEmbedFile`, `renderSoftDeleteFile`, `pendingFile`;
> `cleanup.go` — `errorMapFileName`, `httpFileName`, `openapiFileName`,
> `openapiEmbedFileName`, `softDeleteFileName`, `isCleanupCandidate`;
> `funcs_openapi.go` — the document's YAML shaping helpers;
> `funcs_scope.go` — `isResource`;
> `funcs_softdelete.go` — `softDeleteTypes`; authoritative symbol list:
> `schema_conflicts.go` — `derivedEntityDecls`

## Generated HTTP

`ent.API(client)` returns `*ent.APIHandler`, which is also an `http.Handler`.
Use it directly, mount its endpoints into a consumer mux, or compose it with
ordinary stdlib middleware:

```go
api := ent.API(client)
api.Mount(mux)
mux.Handle("/v1/", http.StripPrefix("/v1", api))
```

### Providing a custom operation implementation

Every reachable operation emits a `{Op}{Entity}Fn` type with exactly the same
signature as its wiring function. `With` accepts only those generated function
types: its `APIOption` method is unexported, and an operation removed by
`Except` emits neither its Fn type nor another way to name that customization
point.

`With` has three fixed laws:

- **Variadic is equivalent to chaining:** `With(a, b).With(c)` is equivalent to
  `With(a, b, c)`.
- **Last wins:** when two options customize the same operation, the later one is
  used.
- **Nil panics at construction:** both a nil `APIOption` and a typed-nil Fn are
  rejected while wiring the handler.

`With` mutates and returns the same `*APIHandler`. Finish wiring before serving;
calling it after requests have begun is a data race and is undefined. A method
value is a small service injection that retains its receiver as a closure:

```go
type ArticleService struct{ patches atomic.Int64 }

func (s *ArticleService) Patch(ctx context.Context, client *ent.Client, id uuid.UUID,
    request *ent.ValidArticlePatchRequest) (*ent.ArticleResponse, error) {
    s.patches.Add(1)
    return ent.PatchArticle(ctx, client, id, request)
}

service := new(ArticleService)
api := ent.API(client).With(ent.PatchArticleFn(service.Patch))
```

Each non-Excepted Resource gets exactly these Go 1.22 patterns:

| Pattern | Result |
|---|---|
| `GET /articles` | bare `{"data","total","page","size"}` page, 200 |
| `POST /articles` | bare resource, 201; no `Location` header |
| `GET /articles/{id}` | bare resource, 200 |
| `PATCH /articles/{id}` | bare resource, 200 |
| `DELETE /articles/{id}` | empty body, 204 |
| `GET /openapi.yaml` | the generated document, 200, `application/yaml` |

Errors are RFC 9457 `application/problem+json`; `WriteProblem` emits
`type: "about:blank"`, title, status and detail, plus `field` when the chain
contains `*FieldError`. Bind failures are 400 except generated `Validate`
failures (422), unsupported media types are 415, oversized bodies are 413, and
middle-step sentinels map to 404/409, plus 400 for List validation and 422 for
Create/Patch validation. Get and Delete have no validation arm. Unclassified
errors are 500. A Save-time Ent `ValidationError` is mapped to 422 and carries
its field name in the problem response when Ent supplies one.

POST and PATCH accept only `application/json`; media-type parameters are
allowed. Their body is capped at **1 MiB before reading, with no configuration
knob**. Unknown keys are compared against the generated create/patch tag data,
so an immutable PATCH key is rejected by name rather than silently discarded.
All three rules live in `entapi.BindJSON`, which a hand-written handler can call
too — see [Bring your own handler](#bring-your-own-handler).

`WithActor` and `ActorFrom` carry authentication state through middleware.

**They travel on the request context, and a generated handler sees nothing
else.** It reads `r.Context()`, so the actor has to be written with
`r.WithContext(entapi.WithActor(...))`:

```go
next.ServeHTTP(w, r.WithContext(entapi.WithActor(r.Context(), user.ID)))
```

A third-party router's own per-request store is a **different** container:
Gin's `c.Set` writes to the `gin.Context`, Echo's `c.Set` to the `echo.Context`.
Neither reaches `r.Context()`, so a generated handler and any customization
point behind it find no actor at all — and because `ActorFrom` reports absence
rather than failing, this surfaces as an actor that is mysteriously nil rather
than as an error. When you authenticate in such a framework's middleware,
replace the request:

```go
func withAuth(c *gin.Context) {
	user, err := verifyToken(c.GetHeader("Authorization"))
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Request = c.Request.WithContext(entapi.WithActor(c.Request.Context(), user.ID))
	c.Next()
}
```

### The generated OpenAPI document

`ent/openapi.yaml` is generated beside the code and committed with it, so the
exposed surface shows up in a pull request diff and is reviewed like anything
else. `entapi_openapi.go` embeds the same bytes and serves them at
`GET /openapi.yaml`, which is why what is on disk and what is served cannot
drift apart.

It is a **derived** document, not a second description: paths and methods come
from the same `resourceOps` the endpoint manifest is built from, so an `Except`ed
operation is absent from both at once; response schemas come from the same
selector the DTOs do, so an Ent `Sensitive()` field cannot reappear in one; and
each filter parameter's operator prefixes come from the field's own generated
allow-list.

Three decisions are worth knowing before you read it:

- **No `servers` entry, and paths carry no prefix.** The mount prefix is a
  deployment fact — `http.StripPrefix` runs in your `main`, long after
  generation, and one build can serve at `/api/v1` and at the bare root at the
  same time. 3.1's default is a relative `/`, which is the one entry that
  cannot lie.
- **Filter parameters are `type: string`** with a `pattern` and a description.
  That is the price of the operator-in-value wire format: `gt:5` is not an
  integer. The description carries what OpenAPI cannot express — the
  operator prefixes the field accepts, and that repeating a parameter ANDs the
  resulting predicates.
- **`GET /openapi.yaml` is in `Endpoints()` but is not described by the
  document.** It is in the manifest so you can wrap or drop it with the same
  loop you use for the CRUD endpoints; it is not in the document because it is not
  part of the resource surface.

The first line is the ownership marker in comment form, and cleanup deletes a
stale document by that marker like any other generated file. Deleting the line
takes the file out of cleanup's deletion candidates, so it survives once the
document stops being generated — it does **not** stop the next generation from
overwriting it, because the writer renames the freshly rendered bytes into place
without ever reading what is there. If you need a `servers` entry, a prefix, or
anything else this generator refuses to guess, do not edit the generated
document: keep your own copy under your own name, build your router from
`Endpoints()` while skipping or replacing the `GET /openapi.yaml` row, and serve
yours there. (`Endpoints()` returns a copy, so `ServeHTTP` and `Mount` still serve
the generated one; the skip has to happen in a router you register yourself.)

**Upgrade hazard:** `ent/openapi.yaml` is an entirely ordinary file name, so a
consumer who already keeps a hand-written document at that path will have it
silently overwritten by the first generation after upgrading — move it aside
before you upgrade.

One honest residue: unlike every other generated file, the document has no
syntax gate before it reaches disk. The standard library has no YAML parser, so
a template bug lands and is caught afterwards — by the fixture assertions and
by the OpenAPI 3.1 validator in `internal/fixtures/httpdemo/e2e`, which is a
nested module precisely so that validator dependency stays out of this one.

### Registering exported endpoints

Composition is a ladder, and which rung you stand on is decided by how much of
the surface you name:

| Rung | Hands you | Reach for it when |
|---|---|---|
| `Get{E}`, `List{Es}`, `Create{E}`, … | the operation, with no HTTP attached | the handler is yours to write |
| `Get{E}Endpoint()`, `List{Es}Endpoint()`, `OpenAPIEndpoint()` | one `entapi.Endpoint`, by name | you register a few endpoints on a router you own, possibly at other paths |
| `Endpoints()` | every endpoint, in registration order | the policy is per batch — "wrap every write", "split by entity" |
| `Mount(mux)`, `ServeHTTP` | the whole tree | the generated paths are the paths you want |

The first rung is the ground the ladder stands on rather than a step on it: a
wiring function is the operation itself, with no HTTP attached and no
`entapi.Endpoint` anywhere. `With` replaces what the generated handlers call,
not what your own code calls, so a direct call to `ListArticles` is unaffected
by it.

The three HTTP rungs above it are one surface, not three: the manifest is built
by calling those same accessors, and `Mount` walks the slice they produce.
Mixing them cannot produce two descriptions of one endpoint. Do not try to
check that by value, though: `Endpoint` is not comparable — `==`, map keys and
`slices.Contains` panic on the handler's func value, and `reflect.DeepEqual` is
always false, because every accessor call constructs a fresh handler value. To
skip or deduplicate rows, key on `Method` plus `Path`, or on `Op`.

#### Taking one endpoint by name

Every reachable operation gets an accessor named after the wiring function it
serves — `GetArticleEndpoint`, `ListArticlesEndpoint`, `CreateArticleEndpoint`,
`PatchArticleEndpoint`, `DeleteArticleEndpoint` — plus `OpenAPIEndpoint` for the
generated document:

```go
api := ent.API(client)
public := http.NewServeMux()

list := api.ListArticlesEndpoint()
public.Handle(list.Method+" /v1/articles", list.Handler)

// A remapped path: Bind feeds the endpoint its own placeholder names, so the
// path you register carries none of them.
featured := api.GetArticleEndpoint()
public.Handle(featured.Method+" /v1/featured", featured.Bind(func(string) string { return featuredID }))

public.Handle("GET /v1/openapi.yaml", api.OpenAPIEndpoint().Handler)
```

The name is what the rung is for. Existence becomes a compile-time fact:
`Except(api.OpDelete)` removes `DeleteAuditLogEndpoint` along with the route, so
a registration naming it stops compiling instead of starting up against an
endpoint that is not there — and jump-to-definition reaches the generated
handler from the registration line. There is deliberately no
`EndpointFor(entity, op)` lookup: a lookup keeps both of those regressions.

An endpoint taken this way is not a snapshot. Its handler reads the current
implementation through the `*APIHandler` at request time, so a `With` call made
after you take it still takes effect.

#### Looping over all of them

`Endpoints()` returns `[]entapi.Endpoint{Method, Path, Handler, Entity, Op}` in
deterministic registration order. Every call returns a fresh slice, so changing
its rows cannot change what `ServeHTTP` or a later `Mount` registers. The rows
are not snapshots, though, for the same reason a single accessor's endpoint is
not one: each carries the handler that reads the current implementation through
the `*APIHandler` at request time, so a `With` call made after `Endpoints()`
returned still takes effect. The list
is a data export, not a mutation API: remove generated endpoints with `Except`,
provide custom implementations with `With`, and register extra endpoints on your
own router.

`Entity` is the Ent type name (`"Article"`) and `Op` is an `entapi.Op` —
`OpList`, `OpCreate`, `OpGet`, `OpPatch`, `OpDelete`, or `OpNone` for an endpoint
that belongs to no resource. They carry the identity the path used to hide, so
splitting the surface by audience is a comparison the compiler checks rather
than a match on path text, where a typo selects nothing and reports nothing.
And because `Endpoint` itself is not comparable, a field comparison like the
`switch` below is the required spelling for picking out a row, not a stylistic
one:

```go
for _, endpoint := range api.Endpoints() {
    switch {
    case endpoint.Entity == "AuditLog":            // internal surface, whole entity
        internal.Handle(endpoint.Method+" "+endpoint.Path, endpoint.Handler)
    case endpoint.Op == entapi.OpNone:             // the document; not a resource
        public.Handle(endpoint.Method+" "+endpoint.Path, endpoint.Handler)
    default:
        public.Handle(endpoint.Method+" "+endpoint.Path, requireScope(endpoint)(endpoint.Handler))
    }
}
```

`Op` is a distinct type rather than a bare string for that reason: it does not
reuse `api.Op`, because the runtime imports no Ent package, but the two are
pinned to the same values by a drift guard.

A complete Gin adapter is consumer code; the framework takes no Gin dependency:

```go
func mountGin(r *gin.Engine, api *ent.APIHandler) {
    for _, endpoint := range api.Endpoints() {
        r.Handle(endpoint.Method, entapi.ColonPath(endpoint.Path), func(c *gin.Context) {
            endpoint.Bind(c.Param).ServeHTTP(c.Writer, c.Request)
        })
    }
}
```

`ColonPath` rewrites whole `{name}` segments to `:name` and leaves every other
segment alone. `Endpoint.Bind` takes a `func(string) string` that exactly matches
`gin.Context.Param` and `echo.Context.Param`. chi and fiber each need a one-line
closure: `chi.URLParam` takes the request too, and `fiber.Ctx.Params` carries a
`defaultValue ...string` variadic, which makes it `func(string, ...string) string`
and therefore not assignable. Echo uses the same two calls,
`entapi.ColonPath(endpoint.Path)` and `endpoint.Bind(c.Param)`, with its response
writer and request.

The placeholder names come from `Endpoint.Path`, so nothing hard-codes `"id"`:
if the generator ever emits a second placeholder, an adapter written this way
picks it up without an edit. `Bind` returns `e.Handler` itself when the endpoint has
no placeholder, so wrapping every endpoint costs nothing for those that do not
need it.

A mount-time constant closure pins an endpoint to a fixed id:
`endpoint.Bind(func(string) string { return actorID })` can serve `/v1/me` from the
same generated `GET /users/{id}` endpoint without a second hand-written wrapper.

One router difference is deliberately not bridged. Go 1.22 `ServeMux` treats
`%2F` as part of one encoded segment and gives the handler a decoded `/` in
`PathValue`; Gin's defaults match the already-decoded `URL.Path`, so
`/articles/a%2Fb` does not match `/articles/:id`. If encoded slashes matter for
an identifier, choose and test the consumer router's policy explicitly.

The metadata also supports selective outer middleware without adding a hook
inside generated handlers. `Op` says what the endpoint does, so "every write" does
not have to be spelled as a list of methods:

```go
for _, endpoint := range api.Endpoints() {
    handler := endpoint.Handler
    switch endpoint.Op {
    case entapi.OpCreate, entapi.OpPatch, entapi.OpDelete:
        handler = requireAuth(handler)
    }
    mux.Handle(endpoint.Method+" "+endpoint.Path, handler)
}
```

Router-level unmatched paths and methods remain the stdlib mux's plain-text
404/405 responses (including `Allow` on 405), not problem+json. This residue is
intentional: installing catch-alls would make mounting into a consumer mux
behave differently from serving the generated tree directly.

### Bring your own handler

Every generated handler is bind → call → write, and all three steps are exported
runtime functions. A hand-written endpoint — on `net/http`, on a third-party
router, for an entity this generator does not own — gets the same behaviour by
calling them instead of copying a generated body:

```go
func BindJSON(w http.ResponseWriter, r *http.Request, tags []string, dst any) error
func Status(err error, onValidation int) int
func WriteJSON(w http.ResponseWriter, status int, v any) error
```

`BindJSON` applies the same three bind rules: the 1 MiB `MaxBytesReader` cap, the
`application/json` media-type check, and rejection of any body key absent from
`tags`. Its error is **total**: every error it returns wraps exactly one of
`entapi.ErrUnsupportedMediaType`, `entapi.ErrRequestTooLarge` or
`entapi.ErrValidation`, so `Status` classifies all of them and there is no fourth
case to write a branch for. It takes `w` only because `http.MaxBytesReader` needs
it, and **writes nothing** — the caller owns the response, including its shape.

`Status` returns 415 and 413 for the two bind sentinels, 404 for
`entapi.ErrNotFound`, 409 for `entapi.ErrAlreadyExists`, `onValidation` for
`entapi.ErrValidation`, 500 for anything else, and 0 for a nil error. The
`onValidation` argument is the whole 400-vs-422 convention: generated handlers
pass **400 for a bind failure** (the request is malformed) and **422 for a
middle-step failure** (the request parsed, and the domain rejected it). Pass the
same two and a custom endpoint answers like a generated one; pass others and it
answers however you chose.

`WriteJSON` marshals before touching the response, so a marshal failure becomes a
clean 500 problem response rather than a truncated 200.

Nothing here is Ent-specific — `entapi/runtime` imports the standard library only
— so this works with your own request type, your own tag list, and your own
response envelope. A Gin endpoint that answers in the caller's envelope rather
than in problem+json:

```go
type createTicketRequest struct {
    Subject string `json:"subject"`
}

var createTicketTags = []string{"subject"}

func (s *server) createTicket(c *gin.Context) {
    var req createTicketRequest
    if err := entapi.BindJSON(c.Writer, c.Request, createTicketTags, &req); err != nil {
        c.JSON(entapi.Status(err, http.StatusBadRequest), envelope{Error: err.Error()})
        return
    }

    ticket, err := s.tickets.Create(c.Request.Context(), req.Subject)
    if err != nil {
        c.JSON(entapi.Status(err, http.StatusUnprocessableEntity), envelope{Error: err.Error()})
        return
    }

    c.JSON(http.StatusCreated, envelope{Data: ticket})
}
```

The generated tag slices (`articleCreateRequestTags`, …) are unexported members
of your `ent` package, so a handler outside it declares its own — which is the
point: `tags` is the caller's allow-list, not a generated one.

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

`Has<Field>()` is generated on **both** the request and its `Valid…` wrapper. A
customization point's signature hands it the validated type and nothing else —
the raw request is unexported inside the wrapper, and the handler has already
consumed the body — so presence that stopped at `Validate` would be unreadable
from the very code that acts on it:

```go
func (s *UserService) Patch(ctx context.Context, db *ent.Client,
	id uuid.UUID, v *ent.ValidUserPatchRequest) (*ent.UserResponse, error) {
	resp, err := ent.PatchUser(ctx, db, id, v)
	if err == nil && v.HasStatus() {          // the wrapper answers, not just the request
		s.mailer.NotifyStatusChange(ctx, id)
	}
	return resp, err
}
```

### Reading the value, not only the presence

Presence is two of the three states. It separates **absent** from **carried**;
it does not separate *carried a value* from *carried an explicit null*, and
those are opposite requests. So every field on the `Valid…PatchRequest` also
has a comma-ok **value reader**, spelled with the field's own name:

```go
func (v *ValidUserPatchRequest) SuspendedUntil() (time.Time, bool)
```

| Reader | `Has<Field>()` | The payload carried | `Apply` will |
|---|---|---|---|
| `ok == true` | `true` | a value | `Set` it |
| `ok == false` | `true` | an explicit `null` | `Clear` it |
| `ok == false` | `false` | nothing | write nothing |

The middle row is reachable only for a clearable field — `Validate` rejects an
explicit null on anything the schema does not declare `Optional()`, so `ok` is
exactly `Has<Field>()` everywhere else. A request built in Go rather than
decoded reads as absent, which is the same answer `Apply` acts on.

That is what lets a cross-field rule be written against the wrapper alone:

```go
if _, ok := v.SuspendedUntil(); ok && status != user.StatusSuspended {
	return nil, fieldError("suspended_until", "only settable while suspended")
}
```

Before the readers, the only way out of the wrapper was `Apply`, so the rule had
to allocate an update builder it never executed, apply the request to it and
read back `Mutation()` — coupling the business logic to Ent's mutation
vocabulary to answer a question about the request (#113).

Only the wrapper gets readers. The **raw** request already exposes its `*T`
fields as exported struct fields, so the value is reachable there; the wrapper
is the only thing that hides it, and the only thing a customization point
receives.

Two field names are refused because of them: a patch-visible field whose Go name
is `Apply`, and a patch-visible pair `x` / `has_x` — see
[generation can fail](#generation-can-fail-and-that-is-the-design).

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
> `WidgetPatchRequestTags()`,
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
vocabulary. `like:`, `ilike:`, `prefix:` and `suffix:` additionally require
`api.Searchable()`. `_ieq` has no wire spelling. Bare `*` and `?` remain
equality literals and do not become implicit `LIKE` patterns.

Parsing follows six ordered rules: an empty bare value is ignored but empty
`eq:` is real; no colon means equality; an allowed prefix applies its operator;
a known but disallowed prefix is validation failure; an unknown prefix falls
back to whole-value equality (which permits bare RFC-3339 timestamps), but under
`WithStrictQueryOperators()` it is a validation failure and such timestamps
must use `eq:`; explicit `eq:` escapes operator-looking values. Conversion
failures name the field and value and wrap `entapi.ErrValidation`.

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

**Every one of them takes a `*Client`, and that is the transaction contract:
this package never generates a transaction boundary.** There is no `*Tx`
variant and no transaction-from-context lookup. To pull a generated step into a
transaction of your own, hand it the client ent already binds to that
transaction:

```go
tx, err := db.Tx(ctx)
// ...
resp, err := ent.PatchUser(ctx, tx.Client(), id, v)   // the generated step, inside your tx
_, err = tx.Client().AuditLog.Create(). /* ... */ .Save(ctx)
err = tx.Commit()
```

Two consequences worth stating plainly. A `*Tx` variant would give every
customization point a twin signature, and "the customization point's signature
is character-for-character the wiring function's" is what makes a wrong
replacement a compile error rather than a runtime surprise. And `ent.API(client)`
holds the root client, so the `db` a custom implementation receives on the HTTP
path is **not** transaction-bound — if you need one there, open it yourself and
call the generated wiring through `tx.Client()`.

No identifier type is hardcoded anywhere — the id comes from your schema's
`$.ID.Type` and reaches the runtime as a type parameter, so an `int` primary key
needs no import at all.

Every exported wiring function returns through `ErrorMap.MapError` **exactly
once**. The file also holds unexported helpers (`{entity}Get`,
`{entity}ByID`, `{entity}Reloaded`) which exist precisely so a create or update
that re-reads through the eager-load plan does not map twice. The result is that
`errors.Is(err, entapi.ErrNotFound)` works at your handler boundary without
unwrapping ent's error types.

`ErrorMap` is emitted with Ent's three generated predicates and a field-name
extractor:

```go
var ErrorMap = entapi.NewErrorMapper(IsNotFound, IsConstraintError).
    WithValidation(IsValidationError, func(err error) (string, bool) {
        var ve *ValidationError
        if errors.As(err, &ve) { return ve.Name, true }
        return "", false
    })
```

The three predicates and `ValidationError` are **unqualified**, so they bind to
symbols Ent generates into the **same package**. That is required: these types
and predicates belong to each consumer project, so the stdlib-only runtime
takes functions and never names an Ent type.

`MapError`'s order is fixed: nil → not-found → validation (with `FieldError`
when the extractor succeeds) → constraint **and** unique → unchanged. The
unique test remains gated behind Ent's `IsConstraintError`; text alone never
classifies an arbitrary error.

`API(client)` installs a dialect-specific unique determination unless
`HasUniqueViolation` says the consumer already installed one:

| Dialect | Recognised as unique | Closed failure to 500 |
|---|---|---|
| `postgres` | An error implementing `SQLState() string` with `23505`; without that method, text containing `violates unique constraint` | A present non-`23505` SQLSTATE is authoritative and never falls through to text. Older lib/pq without `SQLState()` misses under non-English `lc_messages` |
| `mysql` | Text containing `Error 1062` | Other text. The marker is locale-immune because go-sql-driver/mysql formats it from `MySQLError.Number` |
| `sqlite3` | Text containing `UNIQUE constraint failed` | Other text |
| anything else | Nothing; no determination is installed | Every duplicate remains unclassified |

The three text markers are pinned verbatim to Ent v0.14.4's
`sqlgraph.IsUniqueConstraintError`, so text-contract drift is shared with
upstream rather than invented here. Every miss fails closed as 500. A custom
determination installed before `API()` survives auto-wiring; installing one
afterwards overrides it. `ErrorMap` is a plain package-level variable with no
synchronisation, so configure it before serving requests.

One named residue stays deliberately unclassified: Ent reports clearing the
required unique edge `Article.author` as the bare error `wiringent: clearing a
required unique edge "Article.author"`. It is not a `ValidationError`, so it
carries no sentinel and reaches HTTP as 500. The generated PATCH surface cannot
trigger it; only direct builder use can.

> **Implementation:** `templates/wiring.tmpl`, `templates/errors.tmpl`;
> `runtime/errors.go` — `ErrNotFound`, `ErrAlreadyExists`, `ErrValidation`,
> `ErrUnsupportedMediaType`, `ErrRequestTooLarge`,
> `IsNotFound`, `IsAlreadyExists`, `IsValidation`;
> `runtime/bind.go` — `BindJSON`, `Status`, `WriteJSON`;
> `runtime/errors_map.go` —
> `ErrorMapper`, `NewErrorMapper`, `WithValidation`, `WithUniqueViolation`,
> `HasUniqueViolation`, `MapError`; `runtime/errors_dialect.go` —
> `UniqueViolation`;
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

The filter reaches **eager loads too**, not only top-level queries: a
`With<Edge>()` sub-query is an ordinary query on the target's builder and runs
through the same interceptor, so a deleted row does not come back through its
parent either.

Soft delete does **not cascade**. A row whose edge points at a soft-deleted
target keeps its foreign key and stays in every list — nothing tombstones it and
nothing warns. Read together with the line above, those two facts produce one
shape worth knowing before you meet it: an edge declared `Required()` and
expanded with `api.Expand()` comes back as JSON `null` once its target is soft
deleted. That is not a contract violation — `openapi.yaml` documents every
expanded edge as `oneOf [<Target>Summary, null]` — but the schema said the edge
was required, so it reads like one.

**Hand-written code must read edge state through `<Edge>OrErr()`.** After a
plain eager load a soft-deleted target leaves `Edges.X == nil` with **no
error**, which is byte-for-byte what a nil check sees for an edge nobody loaded.
A nil check is what a consumer writes by default, and it silently loses the
distinction:

```go
d, err := client.Draft.Query().Where(draft.ID(id)).WithDoc().Only(ctx)
// ...
target, err := d.Edges.DocOrErr()
switch {
case err == nil:
	// the target is live
case ent.IsNotFound(err):
	// loaded, and there is no row: soft-deleted, or a dangling foreign key
	target = nil
default:
	// never loaded — a query bug, not a data state
	return err
}
```

Generated code already does exactly this; `New<Entity>Response` in
`{entity}_dto.go` is the worked example.

To exclude those rows, filter on the edge in plain ent. An edge predicate is an
ordinary SQL sub-query and carries no traverser of its own, so the tombstone
condition is spelled out:

```go
client.Draft.Query().Where(draft.HasDocWith(doc.DeletedAtIsNil())).All(ctx)
```

> **Proof:** `internal/softdeleteproof/softdelete_test.go` —
> `TestRequiredExpandedEdgeToSoftDeletedTarget` asserts all four against
> real SQLite: the eager-loaded edge is loaded-and-absent rather than not
> loaded, `NewDraftResponse` returns `"doc": null` with no error, the owning
> `Draft` is still listed, and `HasDocWith(DeletedAtIsNil())` excludes it.

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
| An edge Ent marks `Required()` that declares no `edge.Field(…)`, without `Except(OpCreate)` | Ent demands the edge on every create, but no setter for it reaches the create request, so every create fails on `missing required edge`. The remedy list offers `edge.Field(…)` only for an edge that holds the foreign key, which is the only end Ent accepts it on |
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
| **A patch-visible field name colliding with a method the patch DTO generates** | see below |

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

Field names have their own, **method-level** version of the same problem, and
neither check subsumes the other: a method name lives in its receiver's
namespace rather than the package's. The
[value readers](#reading-the-value-not-only-the-presence) are spelled with the
field's own name, so two patch-visible field names now break the build:

| Refused | Collides with |
|---|---|
| a field whose Go name is `Apply` | `Apply(b *<Entity>UpdateOne)` on the same wrapper — `method Apply already declared` |
| a pair `x` and `has_x` | `HasX()`, the presence method generated for `x`, against the struct field `HasX` — `field and method with the same name HasX` |

The second one is **older than the readers**: `Has<Field>()` has been on the raw
request since #98, where `has_x` is a struct field of that very name, so the
pair already failed to compile. Both messages point at `.StorageKey(…)`, because
the JSON tag is spelled from it — the wire key does not have to move when the Go
name does.

`Except(api.OpPatch)` does not exempt either one. It removes endpoints and wiring,
never a request DTO, so the patch request, its wrapper, `Apply` and the readers
are all still generated.

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
> `derivedNameConflict`, `patchMethodCollisions`, `patchApplyCollision`,
> `patchPresenceCollision`, `fieldHasOp`, `markerList`, `errorMapSymbol`,
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
`migrate/`, …) live below the target and are never candidates. Its candidates
are the target's `.go` files plus exactly one name, `openapi.yaml`: that is the
only artifact this extension writes that is not Go source, and widening the
suffix to all YAML would put a consumer's own documents inside the deletion
surface for nothing. A file without
the marker is left alone and **logged** with the reason; ent's own `Code
generated by ent, DO NOT EDIT.` deliberately does not match.

> **The marker line is your escape hatch — from cleanup.** Delete a generated
> file's first line and cleanup stops treating it as a deletion candidate, so
> your copy survives once that file is no longer generated. It does **not**
> protect the file while it still is: every generation renames fresh bytes over
> the path without reading what is there. Conversely, a file you copied out of
> the generated output and forgot to strip the header from **will be deleted**.
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

1. **Unknown dialects do not get a permissive uniqueness guess.** A duplicate
   key stays unclassified and surfaces as 500. Supply `WithUniqueViolation`
   before `API()` for a custom dialect.
2. **`New{E}Response(nil)` returns `(nil, nil)`.** Not an error. Feed it a query
   that matched nothing and you get a pair of nils, not a not-found.
3. **`like:`, `ilike:`, `prefix:` and `suffix:` require `api.Searchable()`.** On a
   Filterable-only string field they are known-but-disallowed operators, so the
   generated parser returns `ErrValidation`.
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
  a summary carries every response-visible field minus the edges. Narrowing it
  needs a new annotation.
- **No optimistic locking, so a lost update is a known boundary.** Generated
  patch wiring is `v.Apply(db.X.UpdateOneID(id))` with no version predicate
  (`templates/wiring.tmpl` — `Patch{Entity}`), so two concurrent `PATCH`es to the
  *same field* end with the later writer winning silently. The exposure is
  narrower than PUT semantics — this package has partial update only, so patches
  touching different fields do not interfere. There is no version word, because
  the framework has no legitimate way to know which column is the version and
  guessing `version`/`updated_at` by name is exactly the convention-derivation
  #18 retired. The escape hatch is the customization point: replace that one step
  with `With(ent.PatchXFn(...))`, add your own `Where(x.Version(v))`, and return
  409 on zero rows affected.
- **Output shares a package with ent's own.** The generator creates no separate
  `dto` subpackage and has no option to change the directory; it establishes
  ownership of the target directory file by file, via the marker, rather than by
  owning a directory outright.
- **Annotations only control HTTP-layer generation.** They never restrict what
  your service layer can do with an ent entity — `Except` closes a handler, its
  endpoint row and a `{Op}{Entity}Fn` type, and leaves the wiring function and
  the request DTO in place. The one exception is a create family that provably
  cannot work (see "The annotation model"). Anything that must be enforced has to
  be enforced where the query is built.
- **The generator package loads all ten templates at package init.** Confine it
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
| §8.4 an `OutputPackage` config option | **Not done**, and moot without §1.6. The supported options configure the runtime import path, strict query parsing, or OpenAPI info; none relocates output |

T2 (the audience dimension), which the design itself deferred, is likewise
unimplemented — consistent with the design.

## Deviations from DESIGN-v3

[`docs/DESIGN-v3-final.md`](docs/DESIGN-v3-final.md) also still says
implementation has not started. That too is **stale**: all eight slices it lists
(#69–#76) have landed, closed by #77, #78, #81, #82, #84, #85, #86 and #87. Read
it for the decisions and their rationale, never for the current API — three
things it specifies were superseded during implementation, and the code is what
shipped:

| Design item | Actual state |
|---|---|
| §2.1 / §2.5 `ent.API(client)` returns `*API`; `func (a *API) Routes()` | The type is **`*APIHandler`** (`templates/http.tmpl` — `API`). `API` is the constructor's name, so the handler could not also be called that. The method is **`Endpoints()`** since #118 — the manifest is a record of handler contracts, not routing |
| §4.3 soft delete registers from a generated `init()`, falling back to an explicit `RegisterSoftDelete(client)` | Neither exists. #78 installs the hook and interceptor from a `config/init/fields/*` **partial that Ent executes inside `newConfig`** (`templates/softdelete_config_init.tmpl`), so `NewClient`, `Open`, `enttest.Open` and every later config copy carry them with no registration call and no initialization-order dependency. `RegisterSoftDelete` was removed rather than kept as a fallback |
| §2.3 the generated handler enables `DisallowUnknownFields`, and the rejected field name is scraped out of `encoding/json`'s error text | The handler decodes the body into a `map[string]json.RawMessage` first and compares its keys against generated `{entity}{Op}RequestTags` data (`templates/handler.tmpl`), reporting the offending key through `entapi.FieldError`. A consumer that writes its own bind step gets the same data from the exported `{Entity}{Op}RequestTags()` accessor, which returns a copy. The design called the error-text scrape a known residue; this removes it — the field name is now generated data, not a parsed string. `DisallowUnknownFields` stays the consumer handler's decision for à la carte DTO decoding |

A fourth item was neither superseded nor already true: the service example
calls `v.HasStatus()` on a validated patch request, and the `Valid…` wrapper
answered no such method. That gap was closed by **generating the forwarder**
rather than by recording a deviation — the example was right about what a
customization point needs, since it receives the validated type and nothing
else. The document's line now compiles.

Everything else in that document — the five deviation words, `Except`'s three
layers plus the create-family exception, the op-in-value wire format, the `_`
namespace, 413/415 request hardening, RFC 9457 errors, the exported manifest,
and the OpenAPI decisions — describes what shipped.

## Migration notes

**Breaking (#118):** `entapi.Route` is now `entapi.Endpoint`, and the generated
`APIHandler.Routes()` is now `APIHandler.Endpoints()`. Nothing else changed —
the fields (`Method`, `Path`, `Handler`, `Entity`, `Op`), the registration
order, the fresh-copy guarantee and `Bind` all behave exactly as before, and
`Op`, its constants and `ColonPath` keep their names. The old name claimed
entapi does routing; it does not. An `Endpoint` is the contract record of one
generated handler — data you compose into a router you own — and the internal
`ServeMux` behind `ServeHTTP`/`Mount` is an optional convenience built from that
same manifest. Regenerate, then rename call sites: `api.Routes()` →
`api.Endpoints()` and `[]entapi.Route` → `[]entapi.Endpoint`.

**Additive, with one upgrade hazard (#119):** the generated `APIHandler` now
carries one exported `…Endpoint()` method per reachable operation (the wiring
function's name plus `Endpoint`: `GetArticle` → `GetArticleEndpoint`), plus
`OpenAPIEndpoint()`. Those names land in your `ent` package's method set. If
you hand-wrote a helper of the same name on `*APIHandler` — the pre-#119
workaround for taking one endpoint by identity was exactly such an index —
regeneration makes it a duplicate-method compile error; delete your copy and
call the generated accessor.

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
| `entapi.Route`, `Route.Bind`, generated `Routes()` | `entapi.Endpoint`, `Endpoint.Bind`, generated `Endpoints()` — same fields, same order, same behaviour (#118) |
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
