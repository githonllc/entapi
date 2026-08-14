# EntAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entapi.svg)](https://pkg.go.dev/github.com/githonllc/entapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entapi)](https://goreportcard.com/report/github.com/githonllc/entapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An [Ent](https://entgo.io) extension. Mark an entity with `api.Resource()` and
it writes request types, response types, a query surface, one wiring function
per operation, a stdlib HTTP route tree and an OpenAPI 3.1 document — all into
your own `ent` package, against a runtime that imports nothing but the standard
library. Field shape comes from Ent; annotations name only deviations.

*[中文](README_zh.md)* · *[Full guide](docs/GUIDE.md)*

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
multi-key sort allow-list, `ArticleResponse` with its eager-load plan, error
classification, five three-step handlers, `openapi.yaml`, and the endpoint
manifest behind `ent.API(client)`.

> ### Status: v0, never released
>
> `git tag` is empty — this repository has never been tagged and has never
> promised an API to anyone. The versioning policy is Go's own `v0.x`
> convention: **break freely, no deprecation window, no compatibility aliases.**
>
> The code itself is complete; known defects are in
> [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md).
>
> Read the [gotchas](#gotchas) before adopting. Several of them are silent.

---

## Contents

- [Quick start](#quick-start) — two runnable services in this repository
- [Install](#install) · [Three import paths](#three-import-paths)
- [The annotation model](#the-annotation-model) — Ent facts plus five deviation words
- [What gets generated](#what-gets-generated) · [Generated HTTP](#generated-http)
- [The query surface](#the-query-surface) · [Requests and responses](#requests-and-responses)
- [Soft delete](#soft-delete) · [Generation can fail](#generation-can-fail-and-that-is-the-design)
- [Gotchas](#gotchas) · [Limits](#limits)
- **[The full guide](docs/GUIDE.md)** — every rule below, with its reason and its source pointer

---

## Quick start

Two complete services live in this repository. Each is one ent schema, one
`go generate`, one `go run`, with nothing hand-written between the client and
the socket:

```bash
cd examples/todo        # one entity, five endpoints
go generate ./...
go run .
```

```bash
cd examples/petstore    # four entities: edges, soft delete, an Excepted operation
go generate ./...
go run .
```

Both listen on `http://localhost:8080`, run on an in-memory SQLite database, and
print their own endpoint manifest at startup — that list comes from
`api.Endpoints()`, not from a hand-written table:

```
GET    /todos
POST   /todos
GET    /todos/{id}
PATCH  /todos/{id}
DELETE /todos/{id}
GET    /openapi.yaml
listening on http://localhost:8080
```

The todo service's entire API surface comes from four annotated fields:

```go
field.String("title").Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
field.Bool("done").Optional().Default(false).Annotations(api.Filterable()),
field.Int("priority").Optional().Default(0).Annotations(api.Filterable(), api.Sortable()),
field.Time("created_at").Immutable().Default(time.Now).Annotations(api.Sortable(), api.ReadOnly()),
```

[`examples/todo/README.md`](examples/todo/README.md) walks every endpoint with
`curl` transcripts pasted from a real run;
[`examples/petstore/README.md`](examples/petstore/README.md) does the same for
the multi-entity case, and
[`examples/petstore/ARCHITECTURE.md`](examples/petstore/ARCHITECTURE.md) maps
which generated file answers which request.

## Install

```bash
go get github.com/githonllc/entapi
```

`go.mod` declares `go 1.23`. The only direct dependencies outside
`golang.org/x` are `entgo.io/ent v0.14.4` and `github.com/google/uuid v1.3.0`.

Install the extension in your `entc.go`:

```go
ext := entapi.NewExtensionWithOptions()

if err := entc.Generate("./schema", &gen.Config{
    Target:  "../ent",
    Package: "your/module/ent",
}, entc.Extensions(ext)); err != nil {
    log.Fatal(err)
}
```

There are four options: `WithEntAPIPackage` (the runtime import path generated
files use — the default is already correct unless you vendored a copy),
`WithStrictQueryOperators()` (an unrecognised operator prefix becomes a
validation failure), and `WithOpenAPITitle` / `WithOpenAPIVersion`.

→ [Wiring it in](docs/GUIDE.md#wiring-it-in)

## Three import paths

One module, three packages, split by *when the code runs*. The root and runtime
packages are named `entapi`; the schema package is named `api`.

| Import | Imported by | Principal symbols |
|---|---|---|
| `github.com/githonllc/entapi` | your `entc.go`; schemas that embed soft delete | `Extension`, `SoftDeleteMixin` |
| `github.com/githonllc/entapi/api` | your **schema** files | `Resource`, `Hidden`, `ReadOnly`, `Searchable`, `Filterable`, `Sortable`, `Expand` |
| `github.com/githonllc/entapi/runtime` | **generated code** and your handler / service code | `ListRequest`, `SortSpec`, `Page[R]`, `ListPage`, `GetOne`, `SaveOne`, `BindJSON`, `Status`, `WriteJSON`, `WriteProblem`, `FieldError`, `Endpoint`/`Op`, `WithActor`/`ActorFrom`, error sentinels and mapper, filter/pointer/soft-delete helpers |

The split is load-bearing. The root package embeds ten templates and reads them
all out of the embedded filesystem **at package init**, dragging in `embed`,
ent's codegen packages and `golang.org/x/tools/imports`. `runtime/` imports the
standard library only, which is what lets it into your production binary while
the root package stays out.

→ [Three import paths](docs/GUIDE.md#three-import-paths)

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

Edges are silent too, and are selected by their own annotation — never inferred
from foreign-key placement:

```go
edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
    Annotations(api.Expand().JSONKey("writer"))
```

`Expand()` puts `Author *UserSummary` in `PostResponse` and `WithAuthor()` in
the generated eager-load plan; `JSONKey("writer")` overrides the response key.
Expansion is one level deep.

The five field words share one mergeable annotation, so separate spelling is
canonical and safe: `Annotations(api.Searchable(), api.Sortable())` preserves
both words through Ent's serialized schema loader.

→ [The annotation model](docs/GUIDE.md#the-annotation-model) ·
[Edges](docs/GUIDE.md#edges)

## What gets generated

Four files per entity carrying **`api.Resource()`**. An entity without that
single switch is skipped entirely.

| File | Declares |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`, `{E}PatchRequest`, a `Valid…` counterpart and `Apply` for each; `{E}Response`, `{E}Summary` and their constructors; `{E}QueryWithResponseEdges`; `{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter`, `Parse{E}Query`, `Predicates()`, `{E}SortKeys`, `{E}Order` |
| `{entity}_wiring.go` | `Get{E}`, `List{Es}`, `Create{E}`, `Patch{E}`, `Delete{E}`, `DeleteBatch{Es}` |
| `{entity}_handler.go` | reachable `{Op}{E}Fn` types and three-step bind → call → write handlers |

Plus five files per schema, each with its own emission condition:

| File | Emitted when | Declares |
|---|---|---|
| `entapi_errors.go` | at least one entity produced wiring | `ErrorMap` |
| `entapi_http.go` | at least one entity carries `api.Resource()` | `APIHandler`, `API(client)`, `With`, `Endpoints`, one `…Endpoint()` accessor per reachable operation, `ServeHTTP`, `Mount` |
| `openapi.yaml` | at least one entity produced wiring | the OpenAPI 3.1 document describing every generated endpoint |
| `entapi_openapi.go` | at least one entity produced wiring | the `//go:embed` of that document and the handler serving it |
| `entapi_softdelete.go` | at least one entity embeds `SoftDeleteMixin` | the unexported query traverser and delete hook |

Output lands in **your** `ent` package (`gen.Config.Target`), so it reads as
`ent.CreateArticle`, `ent.ArticleFilter`, `ent.ErrorMap`. That is also why an
entity name can collide with one.

→ [What gets generated](docs/GUIDE.md#what-gets-generated)

## Generated HTTP

`ent.API(client)` returns `*ent.APIHandler`, which is also an `http.Handler`:

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
| `GET /openapi.yaml` | the generated document, 200, `application/yaml` |

Errors are RFC 9457 `application/problem+json`. Bind failures are 400 except
generated `Validate` failures (422), unsupported media types 415, oversized
bodies 413, and middle-step sentinels map to 404/409. Unclassified errors are
500. POST and PATCH accept only `application/json` and cap the body at **1 MiB
with no configuration knob**.

**Composition is a ladder**, and which rung you stand on is decided by how much
of the surface you name:

| Rung | Hands you | Reach for it when |
|---|---|---|
| `Get{E}`, `List{Es}`, … | the operation, with no HTTP attached | the handler is yours to write |
| `Get{E}Endpoint()`, `OpenAPIEndpoint()` | one `entapi.Endpoint`, by name | you register a few endpoints on a router you own |
| `Endpoints()` | every endpoint, in registration order | the policy is per batch — "wrap every write" |
| `Mount(mux)`, `ServeHTTP` | the whole tree | the generated paths are the paths you want |

The three HTTP rungs are one surface, not three: the manifest is built by
calling those same accessors. `entapi.ColonPath` and `Endpoint.Bind` adapt a row
to Gin, Echo, chi or fiber in one line each; `entapi.BindJSON`, `Status` and
`WriteJSON` give a hand-written handler the same three steps a generated one
uses.

To replace an operation's behaviour without touching its route, pass the
generated function type to `With`:

```go
api := ent.API(client).With(ent.PatchArticleFn(service.Patch))
```

`With` accepts only those generated types, so an operation removed by `Except`
cannot be named at all — a wrong replacement is a compile error. Finish wiring
before serving.

→ [Custom implementations](docs/GUIDE.md#providing-a-custom-operation-implementation) ·
[The OpenAPI document](docs/GUIDE.md#the-generated-openapi-document) ·
[Generating a client with ogen](docs/GUIDE.md#generating-a-client-with-ogen) ·
[Registering endpoints](docs/GUIDE.md#registering-exported-endpoints) ·
[Bring your own handler](docs/GUIDE.md#bring-your-own-handler)

## The query surface

```go
filter, req, err := ent.ParseArticleQuery(r.URL.Query())
```

The wire is `field=op:value`, split on the first colon; a bare value is
equality. Field names always use the Ent storage key.

```text
?title=ilike:go&score=gt:30&score=le:50&status=in:draft,published&_sort=created_at:desc&_page=2
```

| Spelling | Predicate |
|---|---|
| bare value, `eq:` | equality |
| `ne:` | inequality |
| `gt:` `ge:` `lt:` `le:` | comparisons |
| `in:` `not_in:` | comma-separated membership |
| `like:` `ilike:` `prefix:` `suffix:` | string matching — additionally require `api.Searchable()` |
| `is_null:` `not_null:` | null predicates |
| `from:` `to:` `between:a,b` | inclusive range sugar |

Each field receives only the intersection of Ent's predicates and this
vocabulary. Repeated field parameters are separate `AND`ed predicates. Exactly
four reserved parameters exist — `_q` (an `OR` across searchable fields),
`_sort`, `_page`, `_size` — and aliases or repeats of them are rejected.
`{E}Order` is the single sort allow-list; the primary key is always Filterable
and Sortable and is appended as the final tiebreak. Every parse failure is
`entapi.ErrValidation`, naming the field and the value.

→ [The query surface](docs/GUIDE.md#the-query-surface)

## Requests and responses

A PATCH body has to separate three things a plain struct cannot:

| Payload | Means | `HasNickname()` | `Nickname` |
|---|---|---|---|
| `{}` | leave it alone | `false` | `nil` |
| `{"nickname": null}` | clear it | `true` | `nil` |
| `{"nickname": "sam"}` | set it | `true` | `&"sam"` |

Fields stay `*T`; presence lives in a `present map[string]bool` filled by the
generated `UnmarshalJSON` from the raw key set. Every field on the
`Valid…PatchRequest` also has a comma-ok **value reader** spelled with the
field's own name, because presence alone cannot separate *carried a value* from
*carried an explicit null*. A **create** request cannot express "clear", so an
explicit `null` there is recorded as absent.

Validation is not optional — `Validate()` returns a *different type*, and
`Apply` exists only on that type:

```go
valid, err := req.Validate()          // *ValidArticleCreateRequest
if err != nil { return err }          // wraps entapi.ErrValidation
art, err := ent.CreateArticle(ctx, client, valid)
```

On the way out, `New{E}Response` returns an error and `New{E}Summary` cannot.
The difference is edges: edge state is read through ent's `<Edge>OrErr()`, never
a nil check, so loaded-and-absent is an explicit `null` while not-loaded is an
error naming the edge. **Summaries carry no edges**, which is what bounds
expansion to one level with no depth counter and no visited set.

The wiring functions are free functions — no interfaces, nothing to embed:

```go
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entapi.ListRequest) (*entapi.Page[ArticleResponse], error)
func PatchArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
```

Every one takes a `*Client`, and that is the transaction contract: **this
package never generates a transaction boundary.** Compose with your own by
handing a generated step `tx.Client()`. Every exported wiring function returns
through `ErrorMap.MapError` exactly once, so `errors.Is(err,
entapi.ErrNotFound)` works at your handler boundary without unwrapping ent's
error types.

→ [Three-state presence](docs/GUIDE.md#requests-three-state-presence) ·
[Response, summary and edges](docs/GUIDE.md#response-summary-and-edges) ·
[Wiring and error mapping](docs/GUIDE.md#wiring-and-error-mapping) ·
[Field shapes](docs/GUIDE.md#field-shapes)

## Soft delete

Annotation-based, and enforced at ent's layer rather than in the generated
wiring:

```go
func (Doc) Mixin() []ent.Mixin { return []ent.Mixin{entapi.SoftDeleteMixin{}} }
```

The generated `newConfig` installs one interceptor and one hook per
soft-deletable entity, so there is no registration call and no construction-order
dependency. Deleted rows disappear from **every** read — including
`client.Doc.Query()` calls that touch nothing this package generated, and
including eager loads — and `Delete` becomes an update of the tombstone column.
Two independent context switches opt out per call:

```go
entapi.WithSoftDeleted(ctx)   // reads include deleted rows
entapi.WithHardDelete(ctx)    // this delete is a real delete
```

Soft delete does **not** cascade. A row whose edge points at a soft-deleted
target keeps its foreign key and stays in every list, so an edge declared
`Required()` and expanded with `api.Expand()` comes back as JSON `null` once its
target is soft deleted.

→ [Soft delete](docs/GUIDE.md#soft-delete)

## Generation can fail, and that is the design

The checks run **before** `next.Generate(g)`, so a rejected schema leaves
nothing on disk — not even ent's own output. The whole graph is checked and
every problem is reported at once.

> An annotation that contradicts the ent schema fails generation, reporting both
> facts and the fix. Anything that can be generated correctly is generated, not
> refused.

The refusal matrix is a table of contradictions, among them: `api.Hidden()` with any other
field word; Ent `Sensitive()` with a query word; a required-no-default field
blocked from create; an empty PATCH surface; a `Required()` edge with no
`edge.Field(…)`; a query word while `OpList` is excepted; `api.Expand()`
targeting a non-resource; an entity name colliding with a symbol this extension
generates; and a patch-visible field name colliding with a method the patch DTO
generates on the same receiver.

A generation run is **atomic as a whole**: phase one renders and formats every
file in memory, phase two writes them, so a template bug leaves the previous
run's output intact. Afterwards, cleanup deletes any top-level file whose first
line carries `Code generated by entapi extension` or `Code generated by
entdomain extension` — two spellings, both owned by this extension — and that
this run did not write. **That marker line is your escape hatch:** delete it and
the file is yours.

→ [The refusal matrix](docs/GUIDE.md#generation-can-fail-and-that-is-the-design) ·
[What the generator does to your directory](docs/GUIDE.md#what-the-generator-does-to-your-directory)

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
    nothing.** That `int` is your only way to learn how many existed.
10. **`Page.Size` is the clamped size.** The parser rejects zero and negative
    `_size`, accepts values above 1000, and `ListRequest.Limit()` clamps them.
11. **`ErrorMap` is a plain package-level variable with no synchronisation.**
    Assign it where the client is built, not while serving.
12. **A generated file you copied and edited still carries the marker** —
    cleanup will delete it. Strip the first line.
13. **`ent/openapi.yaml` is an ordinary file name.** A hand-written document
    already at that path is silently overwritten by the first generation after
    upgrading. Move it aside first.

## Limits

- **Offset pagination only**, with one `COUNT` per page. It is correct (the
  primary-key tiebreak guarantees a total order), but depth is O(n) and rows can
  still be skipped or repeated *under concurrent writes*. There is no keyset
  alternative and no cursor type in this package.
- **Summaries are always one level deep**, and carry every response-visible
  field minus the edges. Narrowing that needs a new annotation; nothing in the
  schema says which field is the brief one.
- **No optimistic locking, so a lost update is a known boundary.** Generated
  patch wiring carries no version predicate, so two concurrent `PATCH`es to the
  *same field* end with the later writer winning silently. The escape hatch is
  the customization point: replace that one step with `With(ent.PatchXFn(...))`
  and add your own `Where(x.Version(v))`.
- **Output shares a package with ent's own.** There is no `dto` subpackage and
  no option to change the directory; ownership is established file by file, via
  the marker.
- **Annotations only control HTTP-layer generation.** They never restrict what
  your service layer can do with an ent entity. Anything that must be enforced
  has to be enforced where the query is built.
- **The generated OpenAPI document has no syntax gate before it reaches disk**,
  because the standard library has no YAML parser. A template bug lands and is
  caught afterwards by the fixtures and by the 3.1 validator in
  `internal/fixtures/httpdemo/e2e`.
- **Filter parameters are documented single-valued.** Repeating one to AND
  predicates works against the server but is not expressible in the document, so
  a generated client needs a raw query for it (#135).
- **The generator package loads all ten templates at package init.** Confine it
  to `entc.go` and your schema files; `runtime/` is what keeps it out of your
  binary.

→ [Limits](docs/GUIDE.md#limits), with the reasoning behind each one

## Further reading

| | |
|---|---|
| [`docs/GUIDE.md`](docs/GUIDE.md) | **the complete reference** — every rule above, with its reason and a pointer to the source that makes it true |
| [`examples/todo/`](examples/todo/) | one entity, five endpoints, `curl` transcripts from a real run |
| [`examples/petstore/`](examples/petstore/) | four entities: edges, soft delete, an Excepted operation, and a file-by-file architecture map |
| [`docs/adr/`](docs/adr/) | why the load-bearing decisions are what they are — strict key matching, the primary-key tiebreak, per-run atomicity, marker ownership, operator classification |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | the module map, anchored to source the same way the guide is |
| [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md) | known defects |
| [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) | how to build, test and add a fixture |
| `internal/fixtures/` | compilable evidence for every rule here: one hand-written schema per directory, with its generated output committed |
| [`README_zh.md`](README_zh.md) | 中文文档 |

Every push and pull request runs `make check` on GitHub Actions
([`.github/workflows/check.yml`](.github/workflows/check.yml)) — formatting, `go
vet`, this module's tests and the five nested modules — and then asserts two
things about the tree it leaves behind: that `gofmt -l .` is empty, and that
`git status --porcelain` is empty. The second is the generated-output drift
gate: a clean checkout plus a test run has to leave the committed fixtures
byte-identical.

## License

MIT — see [LICENSE](LICENSE).
