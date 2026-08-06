# EntDomain

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entdomain.svg)](https://pkg.go.dev/github.com/githonllc/entdomain)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entdomain)](https://goreportcard.com/report/github.com/githonllc/entdomain)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An [Ent](https://entgo.io) extension. You annotate the fields of your ent schema
with what the HTTP layer may do with them; it generates the request types, the
response types, the query surface and one wiring function per operation — into
your own `ent/` package, against a runtime that depends on nothing but the
standard library.

*[中文文档](README_zh.md)*

```go
// schema/article.go — you write this
field.String("title").
    Annotations(entdomain.DefaultField().AsSearchable().AsFilterable().AsSortable()),
```

```go
// handler.go — you get this
page, err := ent.ListArticles(ctx, client, filter, req)   // GET /articles?title_contains=go&sort_by=title
art,  err := ent.CreateArticle(ctx, client, validReq)     // POST /articles
```

Between those two lines the generator wrote `ArticleCreateRequest` with
three-state presence, `ArticleFilter` with one parameter per operator ent
derives, a sort allow-list, `ArticleResponse` with its eager-load plan, and
error classification — about 700 lines per entity that you would otherwise
write by hand, and rewrite on every schema change.

> ### Status: prototype under redesign
>
> This works and its test suite is green, but the shape of the API is being
> reconsidered. The direction is recorded in [`DESIGN-v2.md`](DESIGN-v2.md) —
> **none of it is implemented**; everything below describes what exists today.
> Known defects live in [`QUALITY-REVIEW.md`](QUALITY-REVIEW.md).
>
> Read [Traps](#traps) before adopting this. Several of them are silent.

---

## Contents

- [Install](#install) · [The two import paths](#the-two-import-paths) · [Setup](#setup)
- [The annotation model](#the-annotation-model) — scopes and markers are different axes
- [What gets generated](#what-gets-generated)
- [Requests: three-state presence](#requests-three-state-presence)
- [Responses, summaries and edges](#responses-summaries-and-edges)
- [The query surface](#the-query-surface) — filters, free text, sorting
- [Wiring and error mapping](#wiring-and-error-mapping)
- [Soft delete](#soft-delete)
- [Generation can fail, and that is the point](#generation-can-fail-and-that-is-the-point)
- [What the generator does to your directory](#what-the-generator-does-to-your-directory)
- [Field shapes](#field-shapes) · [What is accepted but not consumed](#what-is-accepted-but-not-consumed)
- [Traps](#traps) · [Limits](#limits) · [Where else to read](#where-else-to-read)

---

## Install

```bash
go get github.com/githonllc/entdomain
```

Requires Go 1.23+ and ent v0.14+.

## The two import paths

One module, two packages, split by **when the code runs**:

| Import | Imported by | Pulls in |
|---|---|---|
| `github.com/githonllc/entdomain` | your `entc.go` and your **schema** files — annotation builders, `Edge()`, `SoftDeleteMixin`, the extension itself | ent's codegen packages, the source formatter, five embedded templates |
| `github.com/githonllc/entdomain/runtime` | **generated code** and your service/handler code — `ListPage`, `GetOne`, `SaveOne`, `ListRequest`, the error sentinels, `ErrorMapper`, the soft-delete context switches | the standard library, and nothing else |

Both are `package entdomain`, so every call site reads `entdomain.X` whichever
path it arrived by; a file that needs both imports both and aliases one.

The split is load-bearing, not tidiness. Template loading happens at package
init — importing the root package runs it whether or not you generate
anything. Keeping that out of your production binary is the whole reason
`runtime/` exists, and a test (`TestRuntimePackageIsGeneratorFree`) fails the
build if anything ent-shaped, `embed`-shaped or formatter-shaped ever reaches
it. If you add code that generated output calls at run time, it belongs in
`runtime/` and may import stdlib only.

## Setup

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/githonllc/entdomain"
)

func main() {
    ext := entdomain.NewExtensionWithOptions(
        // The RUNTIME path — this is what generated files import.
        // It is also the default, so this line only matters for a vendored copy.
        entdomain.WithEntDomainPackage("github.com/githonllc/entdomain/runtime"),
    )

    if err := entc.Generate("./schema", &gen.Config{
        Target:  "../ent",
        Package: "your/module/ent",
    }, entc.Extensions(ext)); err != nil {
        log.Fatal(err)
    }
}
```

`WithEntDomainPackage` is the only option there is. `NewExtension(cfg)` takes a
`*ExtensionConfig` directly and is nil-safe. Then `go generate ./...`.

## The annotation model

Two axes, and confusing them is the most common mistake:

**Scopes** answer *which HTTP structs may carry this field*. Four of them:
`ScopeCreate`, `ScopeUpdate`, `ScopeQuery`, `ScopeResponse`.

**Markers** answer *what the query API may do with a field that already has
`ScopeQuery`*. Three of them: `AsFilterable()`, `AsSearchable()`,
`AsSortable()`.

```go
entdomain.DefaultField()                    // create + update + query + response
entdomain.InputOnlyField()                  // create + update          (passwords)
entdomain.OutputOnlyField()                 // query + response         (timestamps, computed state)
entdomain.CreateOnlyField()                 // create + query + response (immutable after creation)
entdomain.IdField()                         // OutputOnly, pre-described
entdomain.AuditLogField()                   // OutputOnly, read-only
entdomain.NewDomainField()                  // no scopes — tracked by ent, absent from every HTTP struct
entdomain.DomainFieldWithScopes(scopes...)  // anything else
```

**No preset grants a marker.** `DefaultField()` gives you an empty
`{Entity}Filter` and an empty sort allow-list until you chain one:

```go
field.String("title").
    Annotations(entdomain.DefaultField().
        AsFilterable().     // structured URL parameters: title, title_neq, title_in, title_prefix, …
        AsSearchable().     // joins the free-text q disjunction, and unlocks the substring operators
        AsSortable()),      // enters {Entity}SortKeys
```

That is deliberate (#27). These markers generate real query parameters and a
real `ORDER BY` allow-list; a permissive default would make essentially every
response-visible column orderable and substring-searchable on a table you never
indexed for it. A marker **without** `ScopeQuery` is a generation error, not a
warning — see [Generation can fail](#generation-can-fail-and-that-is-the-point).

Every builder has a **value receiver and returns a copy**. Chaining works;
mutating in place does nothing.

### Edges

Edges are selected by their own annotation, never inferred from where the
foreign key sits:

```go
func (Post) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
            Annotations(entdomain.Edge().InResponse().As("writer")),
    }
}
```

`InResponse()` puts `Author *UserSummary` into `PostResponse` and
`WithAuthor()` into the generated eager-load plan; `As("writer")` overrides the
JSON key. Deriving this from foreign-key placement was tried and rejected: it
made to-many edges permanently unreachable (`edge.Field()` is nil when the
column lives on the other entity) and it fused "expose `author_id`" with
"expose the nested `author`", which are different decisions.

## What gets generated

Per entity that carries at least one annotated field — an entity with none is
skipped entirely, producing no files:

| File | Declares |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`, `{E}PatchRequest`, their `Valid…` counterparts and `Apply`; `{E}Response`, `{E}Summary` and their constructors; `{E}QueryWithResponseEdges`; `{E}ListResponse` and `New{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter` with `Predicates()`, `{E}SortKeys`, `{E}Order` |
| `{entity}_wiring.go` | `Get{E}`, `List{Es}`, `Create{E}`, `Update{E}`, `Delete{E}`, `DeleteBatch{Es}` |

Plus, per schema, two files written only when they have something to say:

| File | Written when | Declares |
|---|---|---|
| `entdomain_errors.go` | any entity is annotated | `ErrorMap` — the classifier every generated operation returns through |
| `entdomain_softdelete.go` | any entity embeds `SoftDeleteMixin` | `RegisterSoftDelete` plus the query traverser and delete hook |

These land in **your** `ent/` package, so they read as `ent.CreateArticle`,
`ent.ArticleFilter`, `ent.ErrorMap`. That is also why entity names collide with
them — see [reserved names](#generation-can-fail-and-that-is-the-point).

## Requests: three-state presence

A PATCH body must distinguish three things, and a plain struct cannot:

| Payload | Meaning | `HasNickname()` | `Nickname` |
|---|---|---|---|
| `{}` | leave it alone | `false` | `nil` |
| `{"nickname": null}` | clear it | `true` | `nil` |
| `{"nickname": "sam"}` | set it | `true` | `&"sam"` |

`Apply` reads exactly that:

```go
if r.HasNickname() {
    if r.Nickname == nil { b.ClearNickname() } else { b.SetNickname(*r.Nickname) }
}
```

A **create** request has no way to express "clear", so an explicit `null` there
is recorded as *absent* — the field goes unwritten and your schema's
`Default()` applies. That is also what makes an explicit null on a required
create field a "required" error rather than a nil dereference.

Requiredness is checked **by presence, not by the zero value** — `0` and
`false` are values. (Strings are the exception: `== ""` says the same thing.)

### Keys are matched strictly

`encoding/json` fills a struct field on an exact match **or** a
case-insensitive one, while presence is recorded by the raw payload key. Those
two disagreed for every case variant, and the failure was silent: `PATCH
{"Nickname":"sam"}` decoded into the field, `HasNickname()` stayed `false`,
`Apply` wrote nothing, and the update reported success having changed no row.

So `UnmarshalJSON` now **refuses** any key that case-folds to a known tag
without matching it exactly:

```
unknown key "Nickname" (did you mean "nickname"?)   // wraps entdomain.ErrValidation
```

The check runs after the raw decode and before the struct decode, so a rejected
request leaves your receiver untouched. A key that folds to *no* tag is still
ignored — rejecting those is `DisallowUnknownFields`, which stays your
handler's decision. Rationale in [ADR-0001](docs/adr/0001-presence-follows-encoding-json-key-matching.md).

### Validation is not optional

`Validate()` returns a *different type*, and `Apply` exists only on that type:

```go
valid, err := req.Validate()          // *ValidArticleCreateRequest
if err != nil { return err }          // wraps entdomain.ErrValidation
art, err := ent.CreateArticle(ctx, client, valid)
```

There is no exported function that applies an unvalidated request. Skipping
validation is a compile error, by construction.

## Responses, summaries and edges

`New{E}Response` returns an error; `New{E}Summary` cannot. The difference is
edges:

- Edge state is read through ent's `<Edge>OrErr()`, never a nil check —
  `loadedTypes` is unexported, so a nil pointer cannot tell *absent* from
  *not loaded*.
- **Loaded and absent is an explicit `null`** (no edge field is `omitempty`).
  **Not loaded is an error** naming the edge — because silently emitting `null`
  for "I forgot to eager-load" is a bug that reaches your API consumers.
- **Summaries carry no edges.** That is what bounds expansion: there is no
  second level for a cycle to close through, so there is no depth counter and
  no visited set. A three-level tree comes back one level deep.

`{E}QueryWithResponseEdges(q)` applies exactly the eager-load plan
`New{E}Response` needs. Use it, or handle the error.

### The named list shape

`List{Es}` returns `*entdomain.Page[{E}Response]`. `{E}ListResponse` is the
same shape under a non-generic name, because OpenAPI/swaggo-class tooling
cannot express a generic instantiation:

```go
page, err := ent.ListArticles(ctx, client, filter, req)
if err != nil { return err }

// @Success 200 {object} ent.ArticleListResponse
return c.JSON(200, ent.NewArticleListResponse(page))
```

The converter's body is a Go type conversion, which is the point: if
`{E}ListResponse` and `entdomain.Page` ever diverge in field set, type or
order, that line stops compiling in every generated package. (Type conversions
ignore struct tags, so a golden-JSON test guards that half separately.)

## The query surface

Three independent dimensions, each opt-in per field.

### Structured filters — `AsFilterable()`

One parameter per operator **ent** derives for the type; this package never
maintains its own operator table. The wire name is the storage key plus a
suffix, used for both `form:` and `json:`:

| | |
|---|---|
| `_neq` `_in` `_not_in` `_gt` `_gte` `_lt` `_lte` | comparison, from ent |
| `_prefix` | left-anchored `LIKE` — uses the index |
| `_contains` `_icontains` `_suffix` `_ieq` | **substring class, see below** |
| `_is_null` | one `*bool`, collapsing `IsNil`/`NotNil` |

A `string` field marked `AsFilterable()` alone gets ten parameters:

```
ref  ref_neq  ref_in  ref_not_in  ref_gt  ref_gte  ref_lt  ref_lte  ref_prefix  ref_is_null
```

### The substring class needs `AsSearchable()` too

`_contains`, `_icontains`, `_suffix` and `_ieq` are the `LIKE '%x%'` shapes
that defeat a B-tree index — the same cost profile the sort and search gates
exist to withhold. They are emitted only when the field carries
`AsSearchable()` **in addition to** `AsFilterable()`.

`_ieq` is exact-match *semantics* but sits in the expensive class for its
*cost*: `LOWER(x) = LOWER(?)` scans without a functional index, exactly like a
substring match. Rationale in [ADR-0005](docs/adr/0005-contains-operators-gated-by-searchable.md).

> **Upgrading:** a `string` field that was `AsFilterable()`-only silently loses
> its four substring parameters. Form and JSON binding drop unknown keys
> without erroring, so a working `?name_contains=x` becomes an *unfiltered*
> query rather than a 400. Add `AsSearchable()` to restore them — which also
> puts the field into the free-text `q` disjunction, an accepted coupling.

### Free text — `AsSearchable()`

Emitted only if at least one field is searchable: a single `q` parameter,
applied as one `OR` disjunction across every searchable field, `AND`ed with
everything else. Skipped when nil **or empty**. A field that is `AsSearchable()`
but not `AsFilterable()` contributes to `q` and gets no structured parameter of
its own.

### Sorting — `AsSortable()`

`{E}SortKeys` is the allow-list. A `sort_by` outside it is
`entdomain.ErrValidation`, never a silent fallback. **There is no default sort
column** — nothing in your schema says which column is the natural one, so the
generator does not guess.

Determinism is a different question, and it does have a schema-given answer.
Offset pagination over a non-total order is incorrect by construction: rows
repeat or vanish between page 1 and page 2 with **zero concurrent writes**. So
every generated order ends with the primary key:

```go
// sort requested: the tiebreak follows the requested direction
[]OrderOption{by(dir), ByID(dir)}       // skipped when the requested key IS the pk
// nothing requested: deterministic, and not claiming to be a "default sort"
[]OrderOption{ByID(sql.OrderAsc())}
```

Rationale in [ADR-0002](docs/adr/0002-deterministic-pagination-pk-tiebreak.md).

### Paging

`entdomain.ListRequest{Size, Page, SortBy, Order}` — the zero value is usable.
`Limit()` clamps to `[1, MaxPageSize]` (1000) and defaults to 20; `Offset()`
saturates rather than overflowing. Out-of-range sizes are **clamped, never
rejected**: `Validate()` says nothing about `Size` or `Page`. If you want a
4xx, compare against `entdomain.MaxPageSize` yourself.

Pagination is offset-only. The cursor codec was removed (#6); `Page` carries
`Data`, `Total`, `Page`, `Size` and nothing else.

## Wiring and error mapping

Free functions, no interfaces, nothing to embed. If you need different
behaviour, write your own function and stop calling the generated one.

```go
func GetArticle(ctx context.Context, db *Client, id uuid.UUID) (*ArticleResponse, error)
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entdomain.ListRequest) (*entdomain.Page[ArticleResponse], error)
func CreateArticle(ctx context.Context, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)
func UpdateArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
func DeleteArticle(ctx context.Context, db *Client, id uuid.UUID) error
func DeleteBatchArticles(ctx context.Context, db *Client, ids []uuid.UUID) (int, error)
```

No identifier type is hardcoded anywhere — the id comes from your schema and
reaches the runtime as a type parameter, so an `int` key needs no import at
all.

Every exported wiring function returns through `ErrorMap.MapError` **exactly
once**, so `errors.Is(err, entdomain.ErrNotFound)` works at your handler
boundary without unwrapping ent's error types.

**`ErrorMap` does not report uniqueness violations out of the box.** ent's
`IsConstraintError` cannot tell a `UNIQUE` violation from a `FOREIGN KEY` one,
so this is opt-in, per driver:

```go
func init() {
    ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(func(err error) bool {
        var pgErr *pgconn.PgError
        return errors.As(err, &pgErr) && pgErr.Code == "23505"
    })
}
```

Skipping it costs you a 500 where a 409 belonged; it never produces a wrong
409.

## Soft delete

Annotation-based, and enforced at ent's layer rather than in generated wiring:

```go
func (Doc) Mixin() []ent.Mixin { return []ent.Mixin{entdomain.SoftDeleteMixin{}} }
```

```go
client := ent.NewClient(ent.Driver(drv))
ent.RegisterSoftDelete(client)          // exactly once, at construction
```

That registration installs an interceptor and a hook. Deleted rows then
disappear from **every** read — including `client.Doc.Query()` calls that touch
nothing this package generated — and `Delete` becomes an update of the
tombstone column. There is no second write and nothing in the generated wiring
knows soft delete exists.

Two independent context switches let you opt out per call:

```go
entdomain.WithSoftDeleted(ctx)   // reads include deleted rows
entdomain.WithHardDelete(ctx)    // this delete really deletes
```

Neither implies the other.

**A client built without `RegisterSoftDelete` filters nothing and hard-deletes
— including in your tests.**

## Generation can fail, and that is the point

Checks run **before** ent writes anything, so a refused schema leaves nothing
on disk — not even ent's own output. The whole graph is checked and every
problem reported at once. The policy:

> An annotation that contradicts the ent schema fails generation, reporting
> both facts and the fix. Anything that can be generated correctly is
> generated, not refused.

| Refused | Because |
|---|---|
| `Immutable()` field carrying `ScopeUpdate` — which `DefaultField()` grants | ent's update builders iterate `MutableFields`, so `SetX` does not exist and no template can emit a call that compiles. Dropping the field silently would make it vanish from your PATCH API where neither `encoding/json` nor `Validate()` could observe it |
| a marker without `ScopeQuery` | the field would be marked filterable and be unreachable from the query API |
| `AsSearchable()` on a type with no `Contains` | there is no substring predicate to emit |
| `AsFilterable()` on a type with no operators | the filter group would be empty |
| `AsSortable()` on a non-comparable type | ent's order builders skip it, so there is no `ByX` to put in the allow-list |
| `DomainSoftDelete` naming a field the entity lacks, or a non-`Optional` tombstone | ent generates no `DeletedAtIsNil` predicate, so the traverser would not compile |
| a self-referential edge pair annotated on one end only | ent hands a chained `edge.To(…).From(…).Annotations(…)` to the *inverse* builder, so the assoc end silently loses its annotation |
| **an entity named after a symbol this extension generates** | see below |

An entity named `ErrorMap` makes ent emit `type ErrorMap` while
`entdomain_errors.go` emits `var ErrorMap` — `redeclared in this block`, in two
files you never wrote, with nothing naming the cause. Same for
`RegisterSoftDelete`, and for cross-entity collisions: an entity literally
named `ArticleResponse` collides with entity `Article`'s generated response
type. All of these are now refused with a message naming both entities, the
symbol, the file it lands in, and the fix.

The reserved list is deliberately the **maximal** set — conditional emission
does not narrow it. Refusing a name that would not have collided today is the
accepted price of not missing one as the templates grow.

Conversely, `Optional().Nillable()` and named types over slices and maps *are*
generated, because correct output exists for them. See [Field shapes](#field-shapes).

## What the generator does to your directory

**Generation is atomic per run.** Phase 1 renders and formats every file into
memory; phase 2 writes them. Any deterministic failure — a template bug, a
refused schema, an unformattable import — lands in phase 1, leaving the
previous run's output entirely intact. Previously a failure at entity B could
leave entity A's files already replaced, giving you a tree that was a mix of
two generations while ent's own output looked fine.

The honest residue: a hard kill *between* renames in phase 2 is a
millisecond-scale window that remains. Closing it needs directory swaps that
are not atomic across platforms. ([ADR-0003](docs/adr/0003-per-run-atomic-generation.md))

**Cleanup deletes stale files, and owns them by a marker.** After a successful
run, the generator scans the target directory and deletes any `.go` file that

1. carries `Code generated by entdomain extension` on its **first line**, and
2. was not written by this run.

That is how a schema edit no longer breaks your build: delete an entity, and
its `_dto.go`/`_filter.go`/`_wiring.go` go with it instead of lingering as
references to builders ent no longer generates. It also removes
`_base_service.go` / `_base_handler.go` for anyone upgrading past #29.

The scan is **top-level only** — ent's generated subpackages live below the
target directory and are never candidates. Files without the marker are left
alone and logged; ent's own `Code generated by ent, DO NOT EDIT.` deliberately
does not match.

> **Your escape hatch is the marker line.** To keep a generated file as your
> own, delete that first line. Conversely, a file you copied from generated
> output and forgot to strip the header from **will be deleted**.
> ([ADR-0004](docs/adr/0004-cleanup-ownership-by-marker.md))

## Field shapes

How ent's modifiers decide the generated request shape. This is derived from
ent, never from a second opinion — ent decides which setters exist, so any
independently-derived shape shows up as a call to a method that was never
generated.

| ent schema | create field | required? | patch clears on `null`? |
|---|---|---|---|
| `field.String("a")` | `string` | yes | no |
| `field.String("a").Default("x")` | `*string` | no | no |
| `field.String("a").Optional()` | `*string` | no | **yes** |
| `field.String("a").Optional().Nillable()` | `*string` | no | yes |
| `field.String("a").Immutable()` | `string` | yes | *absent from PATCH* |
| `field.JSON("tags", []string{}).Optional()` | `*[]string` | no | yes |

A create field is a pointer exactly when ent can fill it without the caller
(`Optional || Default || Nillable`). `WithRequired(ScopeCreate)` can only *add*
strictness, never subtract it.

On the response side, `Optional` comparable fields go through
`entdomain.PtrOrNil`, `Optional` slices and maps — including **named** types
over them — through `entdomain.PtrNilSafe`, chosen by inspecting the reflect
kind rather than the rendered type name.

`Apply` always emits `if r.X != nil { b.SetX(*r.X) }` and never
`SetNillableX`: ent omits the nillable setter for a type that is already
nillable, so `SetNillableTags` does not exist for an optional `field.JSON`. One
uniform branch is correct for every shape.

## What is accepted but not consumed

Fifteen metadata knobs are stored and reach no template. They are reserved for
OpenAPI spec generation (#17) and kept deliberately, not by accident:

`WithTitle` · `WithDescription` · `WithExample` · `WithFormat` · `WithPattern` ·
`WithRange` · `WithLength` · `WithEnum` · `AsReadOnly` · `AsWriteOnly` ·
`AsDeprecated` · `WithTags` · `WithMetadata`

Each one's godoc opens by saying so, and a test enforces that the disclaimer
and the pending-knob ledger stay in sync in both directions — wiring a knob up
forces deleting its disclaimer in the same commit.

**Consumed today:** the four scopes, `Required`, the three query markers, and
the edge annotation's `Scopes` and `JSONKey`. Everything else is storage.

A related contract one level up: this repo treats dead code as a **test
failure**. A template function nothing calls, a template nothing loads, and a
knob that is neither consumed nor declared pending each fail CI.

## Traps

Ordered by how quietly they hurt.

1. **A client without `ent.RegisterSoftDelete(client)` filters nothing and
   hard-deletes.** Including in tests.
2. **`ErrorMap` never returns `ErrAlreadyExists` until you call
   `WithUniqueViolation`.** A duplicate key surfaces as a 500.
3. **`_contains` now requires `AsSearchable()`.** On upgrade, a filter-only
   string field's substring parameters disappear and the query silently
   returns *unfiltered* results rather than erroring.
4. **No preset grants a query marker.** `DefaultField()` alone yields an empty
   filter struct and an empty sort allow-list.
5. **`DefaultField()` on an `Immutable()` field always fails generation** — it
   grants `ScopeUpdate`. Use `CreateOnlyField()` or `OutputOnlyField()`.
6. **An `Immutable()` field named in a PATCH body is discarded by
   `encoding/json` before any validator runs.** Rejecting it needs
   `DisallowUnknownFields` in your handler; the generator cannot see it.
   (Case-*variants* of valid keys are rejected — genuinely unknown keys are
   not.)
7. **`entdomain.IsNotFound` is not ent's `IsNotFound`.** Generated templates
   call the latter unqualified so it binds to ent's predicate inside your
   package. Qualifying it compiles and silently matches nothing.
8. **Every metadata builder is a no-op.** `WithFormat("email")` validates
   nothing.
9. **Chained self-referential edges lose the assoc end's annotation** — ent
   hands it to the inverse builder. Declare the two ends separately.
10. **`DeleteBatch` returns a count, not an error, for ids that matched
    nothing.** The `int` is the only way to learn how many existed.
11. **`Page.Size` is the clamped size**, and an oversized request is never an
    error.
12. **A generated file you copied and edited keeps its marker** — and cleanup
    will delete it. Strip the first line.

## Limits

- **Offset pagination only**, with a `COUNT` per page. It is now correct
  (total order guaranteed), but it is still O(n) deep and can skip or repeat
  rows *under concurrent writes*. There is no keyset alternative in the
  package.
- **Summaries are one level deep**, always. There is no depth option.
- **Which scalar fields a summary carries is not decidable from the schema**,
  so a summary carries every response-scoped field minus the edges. Narrowing
  it needs a new annotation.
- **Scopes control HTTP struct generation only.** They never restrict what your
  service layer can do with an ent entity. Anything enforced must be enforced
  where the query is built.
- **The generator package loads all five templates at package init.** That is
  confined to `entc.go` and schema files; `runtime/` is what keeps it out of
  your binary.

## Where else to read

| | |
|---|---|
| [`docs/adr/`](docs/adr/) | why the load-bearing decisions went the way they did — strict key matching, the PK tiebreak, run-level atomicity, marker ownership, operator classes |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | module map and diagrams |
| [`DESIGN-v2.md`](DESIGN-v2.md) | where this is going, and which of its own first-draft claims were wrong |
| [`QUALITY-REVIEW.md`](QUALITY-REVIEW.md) | known defects |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | how to build, test and add a fixture |
| [`README_zh.md`](README_zh.md) | 中文文档 |

## License

MIT — see [LICENSE](LICENSE).
