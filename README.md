# EntAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entapi.svg)](https://pkg.go.dev/github.com/githonllc/entapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entapi)](https://goreportcard.com/report/github.com/githonllc/entapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An [Ent](https://entgo.io) extension. You annotate a schema field with what the
HTTP layer may do with it, and it writes the request types, the response types,
the query surface and one wiring function per operation — all into your own
`ent` package, against a runtime that imports nothing but the standard library.

*[中文](README_zh.md)*

```go
// schema/article.go — you write this
field.String("title").
    Annotations(entapi.DefaultField().AsSearchable().AsFilterable().AsSortable()),
```

```go
// handler.go — you get this
page, err := ent.ListArticles(ctx, client, filter, req)   // GET /articles?title_contains=go&sort_by=title
art,  err := ent.CreateArticle(ctx, client, validReq)     // POST /articles
```

Between those two lines the generator wrote `ArticleCreateRequest` with
three-state presence, `ArticleFilter` with one parameter per operator ent
derives, a sort allow-list, `ArticleResponse` with its eager-load plan, and
error classification.

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

- [Install](#install) · [Two import paths](#two-import-paths) · [Wiring it in](#wiring-it-in)
- [The annotation model](#the-annotation-model) — scopes and markers are two different axes
- [What gets generated](#what-gets-generated)
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

## Two import paths

One module, two packages, split by *when the code runs*. Both are named
`entapi`, so every call site reads `entapi.X` whichever one it came from;
a file that needs both imports both and aliases one.

| Import | Imported by | Principal symbols |
|---|---|---|
| `github.com/githonllc/entapi` | your `entc.go` and your **schema** files | `Extension`, `DomainField` and its builders, `Edge()`, `SoftDeleteMixin` |
| `github.com/githonllc/entapi/runtime` | **generated code** and your handler / service code | `ListRequest`, `Page[R]`, `ListPage`, `GetOne`, `SaveOne`, `ErrNotFound`/`ErrAlreadyExists`/`ErrValidation`, `ErrorMapper`, `AppendIf`, `Ptr`/`PtrOrNil`/`PtrNilSafe`, `WithSoftDeleted`/`WithHardDelete` |

The split is load-bearing, not cosmetic. The root package embeds five templates
with `//go:embed` and reads all five out of the embedded filesystem **at package
init**, panicking if one is missing — importing the root package runs that
whether or not you generate anything, and drags in `embed`, ent's codegen
packages and `golang.org/x/tools/imports` behind it. (Parsing happens later, per
render: the loader returns the template source as a `string`.) `runtime/` imports
the standard library only, which is what lets it into your production binary
while the root package stays out.

> **Implementation:** `template_loader.go` — `//go:embed templates/*.tmpl`,
> `templateFS`, `loadTemplate`, `mustLoadTemplate` (returns `string`);
> `template_index.go` — `dtoTemplate`, `filterTemplate`, `wiringTemplate`,
> `errorMapTemplate`, `softDeleteTemplate` (five package-level `var`s, all
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

The extension installs exactly one `gen.Hook`. `Templates()` returns an **empty
slice** — this extension does not use Ent's `GraphTemplate` mechanism; it
renders and writes its own files.

> **Implementation:** `extension.go` — `Extension`, `ExtensionConfig`,
> `NewExtension`, `NewExtensionWithOptions`, `Option`, `WithEntAPIPackage`,
> `defaultEntAPIPackage`, `Hooks`, `Templates`, `Annotations`, `Options`,
> `ConfigAnnotation`

## The annotation model

Two axes. Confusing them is the most common mistake.

**Scopes** answer *which HTTP structs may carry this field*. There are four:

| Scope | Appears in |
|---|---|
| `ScopeCreate` | `{E}CreateRequest` |
| `ScopeUpdate` | `{E}PatchRequest` (and only if the field is also in ent's `MutableFields`) |
| `ScopeQuery` | `{E}Filter` / `{E}SortKeys` |
| `ScopeResponse` | `{E}Response` / `{E}Summary` |

**Markers** answer *what the query API may do with a field that already carries
`ScopeQuery`*. There are three: `AsFilterable()`, `AsSearchable()`,
`AsSortable()`.

```go
entapi.DefaultField()                    // create + update + query + response
entapi.InputOnlyField()                  // create + update           (passwords)
entapi.OutputOnlyField()                 // query + response          (timestamps, computed state)
entapi.CreateOnlyField()                 // create + query + response (write-once)
entapi.IdField()                         // OutputOnly + a canned description + ReadOnly metadata
entapi.AuditLogField()                   // OutputOnly + ReadOnly metadata
entapi.NewDomainField()                  // no scopes — ent tracks it, it appears in no HTTP struct
entapi.DomainFieldWithScopes(scopes...)  // any other combination
```

**No preset grants a marker.** All six preset bodies set only `Scopes`; the
`Searchable` / `Sortable` / `Filterable` booleans stay at their zero value.
Until you chain one on, `DefaultField()` gives you an **empty** `{E}Filter`
struct and an **empty** sort allow-list:

```go
field.String("title").
    Annotations(entapi.DefaultField().
        AsFilterable().     // structured URL parameters: title, title_neq, title_in, title_prefix, …
        AsSearchable().     // joins the free-text q disjunction, and unlocks the substring operators
        AsSortable()),      // enters {E}SortKeys
```

A marker **without** `ScopeQuery` is a generation error, not a warning — see
[Generation can fail](#generation-can-fail-and-that-is-the-design).

Every builder takes its receiver **by value and returns a copy**: chaining
works, mutating in place does not. Slice and map fields are reallocated on
copy, so two chains forked from the same base annotation cannot affect each
other.

> **Implementation:** `annotations.go` — `FieldScope`, `ScopeCreate`,
> `ScopeUpdate`, `ScopeQuery`, `ScopeResponse`, `AllFieldScopes`, `DomainField`,
> `NewDomainField`, `DomainFieldWithScopes`, `DefaultField`, `InputOnlyField`,
> `OutputOnlyField`, `CreateOnlyField`, `IdField`, `AuditLogField`,
> `WithRequired`, `AsSearchable`, `AsSortable`, `AsFilterable`, `copyScopes`,
> `copyEnum`, `copyTags`

### Edges

An edge is selected by its own annotation, never inferred from foreign-key
placement:

```go
func (Post) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
            Annotations(entapi.Edge().InResponse().As("writer")),
    }
}
```

`InResponse()` puts `Author *UserSummary` in `PostResponse` and `WithAuthor()`
in the generated eager-load plan; `As("writer")` overrides the JSON key.
`DomainEdge` has exactly two fields, `Scopes` and `JSONKey`, and only
`ScopeResponse` is read today.

An annotation arrives at codegen either as a `*DomainEdge` or as a
`map[string]interface{}` (when loaded from a serialized schema), so every read
goes through one JSON normalisation. The same holds for field annotations.

> **Implementation:** `annotations_edge.go` — `DomainEdge`, `Edge`, `InResponse`,
> `As`, `hasScope`, `getDomainEdgeAnnotation`, `hasEdgeScope`, `responseEdgeSet`,
> `edgeJSONKey`; `funcs_scope.go` — `getDomainFieldAnnotation`, `hasDomainScope`,
> `isDomainRequired`

## What gets generated

Three files per entity that carries **at least one annotated field**. An entity
with none is skipped entirely and produces no files — the first line of the
generation loop is `if len(domainFields(node)) == 0 { continue }`.

| File | Declares |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`, `{E}PatchRequest`, a `Valid…` counterpart and `Apply` for each; `{E}Response`, `{E}Summary` and their constructors; `{E}QueryWithResponseEdges`; `{E}ListResponse` and `New{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter` with its `Predicates()`, `{E}SortKeys`, `{E}Order` |
| `{entity}_wiring.go` | `Get{E}`, `List{Es}`, `Create{E}`, `Update{E}`, `Delete{E}`, `DeleteBatch{Es}` |

Plus two files per schema, each with its own emission condition:

| File | Emitted when | Declares |
|---|---|---|
| `entapi_errors.go` | at least one entity produced wiring | `ErrorMap` |
| `entapi_softdelete.go` | at least one entity embeds `SoftDeleteMixin` | the unexported query traverser and delete hook |

The soft-delete condition is independent of annotations: an entity with **no
domain fields at all** still enters the traverser's type switch if it embeds
the mixin. The extension also supplies a `config/init/fields/*` partial that
extends Ent's own `client.go`: for each such entity, `newConfig` initializes its
hook and interceptor slices. This partial creates no standalone output file and
renders no bytes for a graph without the mixin.

Output lands in **your** `ent` package (`gen.Config.Target`), so it reads as
`ent.CreateArticle`, `ent.ArticleFilter`, `ent.ErrorMap`. That is also why an
entity name can collide with one — see
[reserved names](#generation-can-fail-and-that-is-the-design).

> **Implementation:** `extension.go` — `generatePerTypeFiles`, `perTypeFileName`,
> `renderDTOFile`, `renderFilterFile`, `renderWiringFile`, `renderErrorMapFile`,
> `renderSoftDeleteFile`, `pendingFile`; `cleanup.go` — `errorMapFileName`,
> `softDeleteFileName`; `funcs_fields.go` — `domainFields`;
> `funcs_softdelete.go` — `softDeleteTypes`; authoritative symbol list:
> `schema_conflicts.go` — `derivedEntityDecls`

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

A rejected request never touches your receiver. Keys that match **no** tag under
folding are still ignored — rejecting those is `DisallowUnknownFields`, and it
stays your handler's call. Rationale in
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

A summary carries **every response-scoped scalar field**, minus the edges — its
scalar half is identical to the response's. Narrowing it needs a new annotation;
nothing in the schema says which field is the brief one.

An edge selected for the response whose target entity has **no domain field at
all** is a generation error: that entity is skipped, so there is no
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

Three independent dimensions, each opt-in per field, behind one outer gate:
`ScopeQuery`.

### Structured filtering — `AsFilterable()`

One parameter per operator **ent** derives for that type. This package keeps no
operator table of its own — only a **naming** table, deciding what each operator's
suffix is called. Wire names are the field's **storage key** plus that suffix,
shared by `form:` and `json:`:

| Suffix | |
|---|---|
| *(none)* | `EQ` — the equality parameter is the bare field name |
| `_neq` `_in` `_not_in` `_gt` `_gte` `_lt` `_lte` | comparisons, from ent |
| `_prefix` | left-anchored `LIKE` — uses an index |
| `_contains` `_icontains` `_suffix` `_ieq` | **the substring class, see below** |
| `_is_null` | one `*bool` collapsing `IsNil`/`NotNil` |

An optional `string` field marked only `AsFilterable()` gets ten parameters:

```
ref  ref_neq  ref_in  ref_not_in  ref_gt  ref_gte  ref_lt  ref_lte  ref_prefix  ref_is_null
```

`IsNil` and `NotNil` collapse into one `*bool` because nullability is **one**
question; two parameters would admit a self-contradicting request, and there is
no honest answer to "is null AND is not null".

An operator ent knows and this package has not named is skipped rather than
emitted under a wrong name. There is no such operator today.

### The substring class also needs `AsSearchable()`

`_contains`, `_icontains`, `_suffix` and `_ieq` are precisely the `LIKE '%x%'`
shapes that defeat a B-tree index — the same cost profile the free-text gate
exists to withhold. They are emitted only when the field **also** carries
`AsSearchable()`.

`_ieq` is exact-match *semantically* but sits in the expensive class for its
*cost*: without a functional index, `LOWER(x) = LOWER(?)` scans exactly like a
substring match. Rationale in
[ADR-0005](docs/adr/0005-contains-operators-gated-by-searchable.md).

### Free text — `AsSearchable()`

Emitted only when at least one field is searchable: a single `q` parameter,
applied as an `OR` disjunction across every searchable field and `AND`ed with
everything else. Skipped when nil **or empty**. A field marked `AsSearchable()`
but not `AsFilterable()` contributes to `q` only and gets no structured
parameters of its own.

An entity that marks nothing gets `type PlainFilter struct{}` and `var
PlainSortKeys = []string{}` — empty, but present, because the wiring signatures
need them.

### Sorting — `AsSortable()`

`{E}SortKeys` is the allow-list and `{E}Order` is the function that turns a
request into ent order options. A `sort_by` outside the allow-list is an
`entapi.ErrValidation`, never a silent fallback. A key that passes is then
**thrown away**: what reaches the query is the order builder ent generated for
that column, looked up in a `map[string]func(...) OrderOption` by an
already-validated key. No caller-supplied string is ever interpolated into SQL.

**There is no default sort column** — nothing in your schema says which column
is the natural one, so generation does not invent one.

Determinism is a separate question, and it does have a schema-given answer.
Offset pagination over a non-total order is wrong by construction: rows can
repeat or vanish between page 1 and page 2 with **zero concurrent writes**. So
every generated order ends with the primary key:

```go
// a sort was requested: the tiebreak follows the requested direction
[]OrderOption{by(dir), ByID(dir)}       // skipped when the requested key IS the primary key
// nothing requested: deterministic, and not claiming to be a "default sort"
[]OrderOption{ByID(sql.OrderAsc())}
```

Rationale in
[ADR-0002](docs/adr/0002-deterministic-pagination-pk-tiebreak.md).

### Pagination

`entapi.ListRequest{Size, Page, SortBy, Order}` — usable at its zero value,
all four fields carrying `form:` and `json:` tags.

- `Limit()`: `Size <= 0` → 20 (`DefaultPageSize`); `Size > 1000` → 1000
  (`MaxPageSize`); otherwise as given. **Clamps, never rejects.**
- `Offset()`: `Page <= 1` → 0; otherwise `(Page-1) * Limit()`, **saturating to
  `math.MaxInt`** on multiplication overflow rather than wrapping negative.
- `Validate()` says **nothing** about `Size` or `Page` — it checks `Order` only.
  If you want an oversized size to be a 4xx, compare against
  `entapi.MaxPageSize` yourself.
- `Page.Size` reports the size **actually used**, so clamping is visible.

Pagination is offset-only. `Page` carries `Data`, `Total`, `Page`, `Size` and
nothing else.

> **Implementation:** `funcs_filter.go` — `queryFields`, `isFilterable`,
> `isSearchable`, `isSortable`, `searchFields`, `filterParam`, `filterParams`,
> `opTagSuffix`, `substringOps`, `nullTagSuffix`, `filterImports`;
> `runtime/types.go` — `ListRequest`, `Validate`, `DefaultPageSize`,
> `MaxPageSize`; `runtime/query.go` — `Limit`, `Offset`, `SortKey`, `Page[R]`,
> `Query[Q,P,O,E]`, `ListPage`; `runtime/filter.go` — `AppendIf`,
> `AppendIfSlice`; `templates/filter.tmpl`; generated example:
> `internal/fixtures/query/queryent/record_filter.go` — `RecordFilter`,
> `Predicates`, `recordSortOptions`, `RecordSortKeys`, `RecordOrder`

## Wiring and error mapping

Free functions. No interfaces, nothing to embed. If you need different
behaviour, write your own function and stop calling the generated one.

```go
func GetArticle(ctx context.Context, db *Client, id uuid.UUID) (*ArticleResponse, error)
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entapi.ListRequest) (*entapi.Page[ArticleResponse], error)
func CreateArticle(ctx context.Context, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)
func UpdateArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
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

Nine situations are detected today:

| Refused | Why |
|---|---|
| An `Immutable()` field carrying `ScopeUpdate` — which `DefaultField()` grants | ent's update builders iterate `MutableFields`, so `SetX` does not exist and no template can emit a call that compiles |
| A marker without `ScopeQuery` | the field is marked filterable/searchable/sortable but is unreachable from the query API, and no query artifact is generated for it |
| `AsSearchable()` on a type with no `Contains` | there is no substring predicate to put in the free-text disjunction |
| `AsFilterable()` on a type with no operators at all | the filter group would be empty and the parameter would silently do nothing |
| `AsSortable()` on a non-comparable type | ent's order builders skip it, so there is no `ByX` for the allow-list |
| `DomainSoftDelete` naming a field the entity does not have | attaching the marker by hand is unsupported; embed `SoftDeleteMixin` instead |
| A tombstone field that is not `Optional` | ent generates no `DeletedAtIsNil` predicate and the traverser would not compile |
| A self-referential edge pair annotated on one end only | ent hands chained `edge.To(…).From(…).Annotations(…)` to the *inverse* builder, so the assoc end silently loses its annotation |
| **An entity name colliding with a symbol this extension generates** | see below |

An entity called `ErrorMap` makes ent emit `type ErrorMap` while
`entapi_errors.go` emits `var ErrorMap` — and Go gives types, variables and
functions **one** identifier namespace per package. The result is `redeclared in
this block`, in two files the author never wrote, with nothing naming the cause.
The same holds across entities: an entity literally named `ArticleResponse`
collides with the response type generated for entity `Article`.

The reserved-name check runs at graph level rather than inside the node loop,
because **the colliding entity does not have to be annotated** — ent generates a
type for every entity, so a bare `type ErrorMap struct{ ent.Schema }` collides
just as hard, and the node loop skips exactly those. The derived list is the
**maximal** set of names an annotated entity can produce (an entity with no
create-scoped field emits no `<Name>CreateRequest` today, but adding one scope
later would), so refusing a name that would not have collided today is the
accepted cost of a refusal that is stable.

Every check except the soft-delete pair and the reserved names **skips an entity
with no domain fields** — the same condition the generation loop uses.

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
`_dto.go`/`_filter.go`/`_wiring.go` go with it, instead of lingering as a
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
- A create field is **required** when the annotation demands it, or when
  `!Optional && !Default`. `WithRequired(ScopeCreate)` can only *add* strictness,
  never remove it.
- A patch field is **clearable** when `Optional &&` the annotation does not mark
  it required for update.

`patchFields` iterates `node.MutableFields()` rather than `node.Fields`, so a
field that survives provably has a `Set<Field>`. (`Immutable` + `ScopeUpdate` is
refused at generation time first, so the intersection currently drops nothing —
the refusal is what the author sees, the filter is what keeps the output
correct.)

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

## Accepted but not consumed

**Fifteen metadata knobs are stored and reach no template**: `DomainField.Metadata`
itself, plus the fourteen fields of `FieldMetadata`. Thirteen builders write
them:

`WithMetadata` · `WithTitle` · `WithDescription` · `WithExample` · `WithFormat` ·
`WithPattern` · `WithRange` · `WithLength` · `WithEnum` · `AsReadOnly` ·
`AsWriteOnly` · `AsDeprecated` · `WithTags`

They are reserved for OpenAPI / Swagger spec generation, deliberately kept
rather than overlooked — the first line of each one's godoc says so. A test
derives the full knob list by reflection, then toggles each knob and renders the
templates to decide reachability: an unreachable knob absent from the pending
ledger fails CI, **and so does a listed knob that has become reachable**. The
ledger is a claim with a deadline, not an exemption.

**Consumed today:** `DomainField.Scopes`, `.Required`, `.Searchable`,
`.Sortable`, `.Filterable`, plus `DomainEdge.Scopes` and `.JSONKey`.

A related contract one level up: this repository treats dead code as a **test
failure**. A template function nobody calls, a template nobody loads, and a knob
that is neither consumed nor declared pending all break CI.

> **Implementation:** `annotations.go` — `FieldMetadata`,
> `DomainField.Metadata`, `ensureMetadata` and the thirteen builders;
> `annotation_surface_test.go` — `pendingKnobs`; `funcs.go` — `templateFuncs`
> (the registry itself: a helper is callable from a template only if it appears
> there)

## Gotchas

Ordered by how quietly they hurt you.

1. **`ErrorMap` never returns `ErrAlreadyExists` until you call
   `WithUniqueViolation`.** A duplicate key passes through `MapError` unchanged
   and surfaces as a 500.
2. **`New{E}Response(nil)` returns `(nil, nil)`.** Not an error. Feed it a query
   that matched nothing and you get a pair of nils, not a not-found.
3. **`_contains` requires `AsSearchable()`.** A string field marked filterable
   only emits none of its four substring parameters; form and JSON binders drop
   unknown keys without complaint, so `?name_contains=x` becomes an *unfiltered*
   query rather than a 400.
4. **No preset grants a query marker.** `DefaultField()` alone gives you an empty
   filter struct and an empty sort allow-list.
5. **`DefaultField()` on an `Immutable()` field always fails generation** — it
   grants `ScopeUpdate`. Use `CreateOnlyField()` or `OutputOnlyField()`.
6. **An `Immutable()` field in a PATCH body is discarded by `encoding/json`
   before any validator runs.** Rejecting it needs `DisallowUnknownFields` in
   your handler; the generator cannot see it. (Case *variants* of legitimate keys
   are rejected — genuinely unknown keys are not.)
7. **`entapi.IsNotFound` is not ent's `IsNotFound`.** The templates call the
   latter *unqualified* so it binds to ent's generated predicate in your package.
   Qualifying it still compiles and then silently matches nothing.
8. **Every metadata builder is a no-op.** `WithFormat("email")` validates
   nothing.
9. **`DeleteBatch` returns a count, not an error, for ids that matched
    nothing.** That `int` is your only way to learn how many existed; an empty
    list deletes zero rows, which is ent's own reading of `IDIn` with no
    arguments rather than a guard written here.
10. **`Page.Size` is the clamped size**, and an oversized request is never an
    error — `ListRequest.Validate()` says nothing about `Size` or `Page`.
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

The following symbols once existed in this module and have been removed, with
**no compatibility aliases** — an alias that preserves the coupling a change
exists to remove is worse than the break.

| Removed | Use instead |
|---|---|
| generated `RegisterSoftDelete` | nothing at client construction — embed `SoftDeleteMixin` in the schema and regenerate |
| `Base{Entity}Service`, `Base{Entity}Handler`, `SetSelf`, generated hooks | the generated free functions (`Get{E}`, `List{Es}`, …); write your own function if you need different behaviour |
| `ExtensionConfig.GenerateBaseService`, `.GenerateBaseHandler`, `WithBaseService`, `WithBaseHandler` | nothing — the base classes are gone |
| `{Entity}EntToResponse` | `New{Entity}Response`, which returns an error rather than nil on failure |
| `Apply{Entity}CreateRequest`, `Apply{Entity}UpdateRequest` (free functions) | `Valid{Entity}…Request.Apply` |
| `Cursor`, `PageInfo`, `EncodeCursor`, `DecodeCursor`, `ListRequest.Cursor` | nothing — pagination is offset-only |
| `DomainField.Sensitive`, `AsSensitive` | mark the field `Sensitive()` in the ent schema — it is then dropped from both response tiers regardless of scope; or withhold `ScopeResponse` |
| `DomainField.UniqueLookup`, `.RangeLookup`, `.Validation` | `AsFilterable()` (operators are derived from ent's `$field.Ops`); `Validate()` |
| `DomainConfig.EntityName` | nothing — it had no readers |
| runtime symbols living in the root package | all moved to `github.com/githonllc/entapi/runtime` |

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
