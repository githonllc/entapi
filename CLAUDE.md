# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make check                  # fmt + vet + test + test-modules (run before committing)
make test                   # go test -count=1 -v ./...  (this module only)
make test-modules           # the five nested modules; NOT covered by make test
make cover                  # coverage summary (CONTRIBUTING targets >85%)
make lint                   # golangci-lint run ./...   (v1; v2 rejects .golangci.yml)
make fmt                    # gofmt + goimports -local github.com/githonllc/entapi

go test -run TestCamelCase -v ./.          # single test, generator package
go test -run 'TestErrorMapper_.*' ./runtime  # subset by regex, runtime package
```

Note: the Makefile overrides `GOPATH=/tmp/gopath` and `GOMODCACHE=/tmp/gomodcache`. Bare `go test ./...` uses your normal module cache and is equivalent otherwise.

`make test` does **not** reach the nested modules — they are separate `go.mod`s, so `./...` never descends into them. `make check` runs both. A change that compiles and passes `make test` can still be caught by `make test-modules`; that is the point of it.

## What this repository is

One Go module, **three packages**, split by *when the code runs* (#15, #71).

1. **`github.com/githonllc/entapi` — code-generation time.** The Ent extension and `SoftDeleteMixin`. It writes `{entity}_dto.go`, `{entity}_filter.go`, `{entity}_wiring.go` per `api.Resource()`, plus graph files, into the consumer's `ent/` package.
2. **`github.com/githonllc/entapi/api` — schema time only.** The three mergeable annotations and their builders. It may import only `entgo.io/ent/schema` and stdlib; `TestSchemaAPIPackageIsGeneratorFree` guards the transitive closure.
3. **`github.com/githonllc/entapi/runtime` — application run time.** The types generated code links against: `Page`/`ListRequest`/`SortSpec`, the generic `ListPage`/`GetOne`/`SaveOne`, `AppendEach`/`AppendEachSlice`, `ParseFieldValues`/`QueryOp` (the table-driven per-field query dispatch), lexical URL-query helpers, error sentinels and mapper, `WriteProblem`/`FieldError`/`Endpoint` plus `ColonPath`/`Endpoint.Bind`, actor context, pointer helpers, and soft-delete context switches.

**The split is load-bearing, not cosmetic.** `template_index.go` declares package-level vars calling `mustLoadTemplate`, so while the two halves shared a package, a consumer's production binary that wanted `ErrValidation` embedded five templates and ran the template loader at init. Measured, and asserted by `TestRuntimePackageIsGeneratorFree`: `go list -deps ./runtime` reports **0** `entgo.io` packages out of 186 (all standard library, the `vendor/golang.org/x/…` entries being the ones the Go distribution ships inside std) and `EmbedFiles` is empty, against **15** for the generator — which is correct there, since generating genuinely needs ent. The closure was 62 packages until #73 put `WriteProblem` in the runtime and brought `net/http` with it; the invariant the test asserts is the **0**, never the total, which is why the growth is a documentation update and not a regression.

The dependency edge runs one way but is not empty: `funcs_openapi.go` imports `github.com/githonllc/entapi/runtime` (as `entapiruntime`, for `DefaultPageSize`/`MaxPageSize` in the `_size` parameter's documented bounds), and it is the first non-test import in that direction. It is correct because the alternative was a second copy of those two constants in the generator, and the document is derived from the runtime's real limits rather than duplicating them; the direction is the safe one, since the runtime still imports nothing from the root.

Keep new code on the right side of that line. Anything the generated output calls at run time belongs in `runtime/` and must import stdlib only; anything ent loads belongs in the root. `softdelete.go` is the worked example of the seam: the mixin (schema-time, imports `entgo.io/ent/schema`) stayed, while `WithSoftDeleted`/`WithHardDelete` and their predicates moved to `runtime/softdelete_context.go` because the generated traverser and hook call them on every query and every delete.

There is no `main` or example app. Anything about *generated* code can only be verified by generating into a real ent project — which `TestCodegenFixtures` and the five nested modules do.

## Generation pipeline

`extension.go` is the whole entry point:

- `Hooks()` returns one `gen.Hook` (`generatePerTypeFiles`). `Templates()` returns exactly one exception: `softdelete_config_init`, a `config/init/fields/*` **partial** that extends Ent's own `client.tmpl` inside `newConfig`. It is not a standalone `GraphTemplate` output and creates no new file; it renders no bytes when the graph has no `SoftDeleteMixin`, so those clients remain byte-identical to plain Ent output. All standalone entapi files still go through the hook below.
- The hook checks conflicts **before** `next.Generate(g)`, then loops `g.Nodes`. **A node without `api.Resource()` is skipped entirely.** Files an earlier run wrote for a node that is now skipped are deleted afterwards; see "Stale artifact removal" below.
- Generation is **two-phase** (#61). Phase 1 renders every file with `text/template` and formats it through `golang.org/x/tools/imports` in `formatFile`, collecting `[]pendingFile` in memory; phase 2 writes them with `writeFormatted` (temp file in the target directory, renamed into place). **Formatting failure aborts generation and writes nothing** — `imports.Process` only fails on source it cannot parse, so its failure is a template bug, and because it is a pure function of the bytes it lands in phase 1, before anything is on disk. That is what makes the run atomic rather than merely each file: a failure at entity B can no longer leave entity A's files already replaced. The residue, stated honestly in `generatePerTypeFiles`, is a hard kill inside the phase-2 rename loop. `writeFile` no longer exists.
- Templates declare the imports their output uses (`dtoImports` in `funcs_imports.go` computes the field-type ones). goimports stays in the pipeline as a safety net, not as the mechanism — it cannot be the mechanism now that its failure is fatal. `TestTemplatesDeclareTheirImports` fails if goimports has to add or remove anything.
- `templateFuncMap()` layers three sources, later wins: Ent's `gen.Funcs` → this package's `templateFuncs()` → the configuration closures `entapiPkg`, `strictQueryOperators`, `openapiTitle` and `openapiVersion`. Closures are exempt from `TestTemplateInvocationsAreRegistered`'s reverse direction (it reads `templateFuncs()` only), so a genuine helper belongs in `templateFuncs()` and only configuration belongs here.

The full order inside the hook is: `checkGraphConflicts` (`schema_conflicts.go`, refuses contradicting annotations *before* ent writes anything) → `next.Generate(g)` → the per-node loop → stale artifact removal.

### Stale artifact removal

`cleanup.go` runs after every file of a successful run is on disk and deletes what this extension wrote earlier but did not write this time. It deletes files from the consumer's repository, so all three fences have to hold:

1. only **top-level** files of the target directory — `os.ReadDir`, never a walk. ent's generated subpackages (`<entity>/`, `predicate/`, …) live below that directory and are never candidates (#63). `isCleanupCandidate` narrows those to `.go` plus the single exact name `openapi.yaml`, the only artifact this extension writes that is not Go source (#76); the exception is a name, not a `.yaml` suffix class, so a consumer's own marked YAML stays out of the deletion surface;
2. only files whose **first line** carries `Code generated by entapi extension` — deliberately narrower than ent's own `Code generated by ent, DO NOT EDIT.`;
3. never a path this run just wrote.

Fence 1 used to enumerate names instead — `generatedFileNames(node)` / `graphFileNames()`, restricted to entities present in *this* run's graph. Both are gone (#63): an entity **deleted** from the schema has no node, so its marker-bearing files were never candidates and broke the consumer's build. The scan subsumes the enumeration, including the two legacy `_base_service.go` / `_base_handler.go` names, which carry the marker like anything else this extension ever wrote.

**The marker line is the ownership contract**, and it is the consumer's escape hatch: delete that first line and the file becomes theirs, permanently.

Candidates that fail fence 2 are left alone and logged. Cleanup never runs on a failed generation, so neither a template bug nor a refused schema can take the previous output with it.

### Templates

Ten live templates, all embedded and all bound: `dto`, `filter`, `wiring`, `handler` (rendered per `*gen.Type`); `errors`, `http`, `openapi`, `openapi_embed`, `softdelete` (rendered once per `*gen.Graph`); and `softdelete_config_init`, the partial Ent executes inside `newConfig`. `openapi` is the one whose output is not Go — see "The OpenAPI document" below. `TestEveryEmbeddedTemplateIsLoaded` requires every embedded `.tmpl` to be bound in `template_index.go`, so there is no such thing as an embedded-but-unused template here.

`templates/*.tmpl` are embedded via `//go:embed` (`template_loader.go`) and loaded into package-level vars by `template_index.go` using `mustLoadTemplate`, i.e. **at package init — renaming or deleting a template panics on import**, not at generation time. That init is exactly what `runtime/` exists to stay out of; keep the embed directive and `templates/` in the generator package.

Per-type templates receive a `*gen.Type` as `.`, so `$.Config.Package`, `$.Package`, `$.Name`, `$.ID` and the standard Ent template funcs are all available. The five graph-level output templates and the config partial receive a `*gen.Graph`, so they see `$.Nodes` instead.

**Every standalone-output Go template that names a runtime symbol imports the runtime under an explicit `entapi` alias**, and the alias is required rather than decorative: the path's last element is `runtime`, so goimports would read an unaliased import as package `runtime`, find no use of that name, and delete it — and a formatter failure aborts the whole run in phase 1. The config partial declares no imports; it names only helpers in the generated package.

### Template functions

Split by concern across `funcs_*.go`, and registered in one map in `funcs.go`:

| File | Contains |
|---|---|
| `funcs_fields.go` | derived field/edge selection: `createFields`, `patchFields`, `responseFields`, `responseEdges` |
| `funcs_scope.go` | normalized `getResourceAnnotation`/`getFieldAnnotation`/`getEdgeAnnotation` readers, `isResource`, `resourceExcepts`, `hasCreateFamily` |
| `funcs_presence.go` | request field shape: `isCreatePointer`, `isCreateRequired`, `isPatchClearable` |
| `funcs_typechecks.go` | `isComplexFieldType` — **not registered as a template func**; only `fieldValueExpr` calls it |
| `funcs_softdelete.go` | `isSoftDeletable`, `softDeleteTypes`, `softDeleteField`, `softDeleteImports` — the soft-delete mixin's marker |
| `funcs_imports.go` | `dtoImports`: the import specs the DTO must declare for its field types |
| `funcs_codegen.go` | `fieldValueExpr` — the response constructor's per-field expression, and the only caller of `isComplexFieldType` |
| `funcs_strings.go` | `camelCase`, and nothing else — Ent's `gen.Funcs` already supplies `contains`, `hasPrefix`, `lower`, `snake`, `plural`, … |
| `funcs_filter.go` | the query surface: `queryFields`, `parseFields` (ID plus Filterable fields), `searchFields`, `isSortable`, per-field wire operator sets and conversion expressions, `filterImports` |
| `funcs_http.go` | reachable operations, route paths, path-ID parsing and handler imports |
| `funcs_openapi.go` | the OpenAPI document's YAML shaping: `yamlQuote`, `openapiSchema`, `openapiPathGroups`, `openapiFilterParams`, `openapiReservedParams`, `openapiErrorStatuses`/`openapiProblemStatuses`, `openapiRequiredCreateFields` |
| `annotations_edge.go` | `responseEdgeSet` and `edgeJSONKey`, reading `api.Expand()` through the normalized edge reader |
| `api/annotations.go` | schema-time `ResourceAnnotation`, `FieldAnnotation`, `EdgeAnnotation`; every type implements `schema.Merger` |

**A helper is only callable from a template if it appears in `templateFuncs()`.** Adding a func to a `funcs_*.go` file is not enough.

### Query wire format (#72)

`templates/filter.tmpl` emits `Parse{Entity}Query(url.Values)`, the typed filter,
and `{Entity}Order`. Runtime owns only lexical grammar: split on the first colon,
the global operator-prefix vocabulary, reserved parameter syntax and sort-spec
parsing. Generated code owns semantic permission and conversion: each field's
allowed operators are `$field.Ops` intersected with the wire vocabulary, with
Searchable gating the expensive substring class. Wire field names always come
from `StorageKey()`. The primary key is annotation-free but always Filterable
and Sortable; its predicates use Ent's `ID…` prefix and its order uses
`$.ID.OrderName`. Repeated field values become separate ANDed predicates;
reserved parameters remain single-valued. `{Entity}Order`, not the parser, is
the only sort-key allow-list check and appends the ID tiebreak only when ID is
absent from the entire sort list. By default an unknown operator prefix falls
back to whole-value equality; `WithStrictQueryOperators()` opts into validation
failure instead, which means colon-bearing literals — including bare
RFC-3339 timestamps — must use an explicit `eq:` prefix.

Every parse failure is `ErrValidation`, i.e. a 400 that names the field and the
value — a disallowed operator prefix, an unparseable value, a non-member enum, a
non-RFC-3339 time. Silently dropping the predicate would be the fail-open
direction: the response would carry MORE rows than the caller asked for. One
wire value is deliberately reserved rather than clamped: an explicit `_size=0`
is a 400 (`runtime/urlquery.go` — `ParsePageParam`), because 0 is held for a
future count-only mode and clamping it first would break consumers later.

### The OpenAPI document (#76)

`templates/openapi.tmpl` renders `openapi.yaml` and
`templates/openapi_embed.tmpl` renders `entapi_openapi.go`, which `//go:embed`s
it and serves it from the unexported `serveOpenAPI`. `templates/http.tmpl`
appends `GET /openapi.yaml` to `h.endpoints` **last**, so it is visible to
`Endpoints()` and wrappable with the same loop as any CRUD endpoint. Four things are
load-bearing:

- **`renderOpenAPIFile` is the one render\*File that skips `formatFile`.** That
  is forced, not chosen: `imports.Process` parses its input as Go, and a
  formatting failure aborts the whole run in phase 1. The honest consequence,
  written into the template header and both READMEs, is that this file alone
  has no syntax gate before it reaches disk.
- **The document is derived, and nothing in `funcs_openapi.go` may be a second
  opinion.** Paths and methods come from `resourceOps`/`routePath` — the same
  source the endpoint manifest uses; schemas from
  `responseFields`/`responseEdges`/`createFields`/`patchFields`; filter
  parameters from `parseFields` and the per-field operator sets in
  `funcs_filter.go`. A field, an operation or an operator spelled out in the
  template or in a test is a defect, not a shortcut. The one table that IS here
  is `errorStatusesByOp`, probed from `runtime.Status`'s behaviour together with
  the template's call sites — `handler.tmpl` stopped spelling most statuses as
  literals in #103, so `openapi_status_drift_test.go` now derives each branch's
  expected set by calling `runtime.Status` over a hand-chosen sentinel list at
  each `entapi.Status`/`entapi.BindJSON` site's `onValidation` argument, and
  unions that with the literals still in the text;
  `openapiProblemStatuses` is its union rather than a second list.
- **No YAML library, in either module.** `yamlQuote` is the whole mechanism:
  YAML 1.2 is a strict superset of JSON, so a JSON string literal is a YAML
  double-quoted scalar and a JSON flow mapping is a YAML mapping. Every
  data-derived scalar goes through it, **including map keys** — a storage key
  spelled `on` or `null` is a boolean to a YAML 1.1 parser if left bare.
- **Iteration is `$.Nodes`, ent field order and ent operator order, never a Go
  map.** The output is committed, so a map range would make `TestCodegenFixtures`
  dirty the tree on alternate runs.

`_size` documents `minimum: 1` and **no `maximum`**: `ParsePageParam` accepts a
larger value and `ListRequest.Limit` clamps it, so a `maximum` keyword would
document a 400 the handler never returns. The bound is in the description
instead, read from `runtime.MaxPageSize` rather than restated.

### Annotation access

Never read the EntAPI annotation maps directly. Always use the three readers in `funcs_scope.go`: annotations arrive as Go values in hand-built unit graphs but as `map[string]interface{}` through serialized schema loading, and the readers normalize both via a JSON round-trip.

### Create and patch requests (#26)

`dto.tmpl` emits, per entity: `{Entity}CreateRequest`, `{Entity}PatchRequest`,
a `Valid…` wrapper for each, `{Entity}CreateRequestTags()` and
`{Entity}PatchRequestTags()` accessors, an `UnmarshalJSON`, one `Has<Field>()`
per field on **both** the request and its wrapper, one comma-ok value reader
`<Field>() (T, bool)` per field on the **patch wrapper only**, and `Apply` on the
validated types only.
The wrapper's `Has<Field>()` forwards; it exists because a customization point
receives only the validated type, so presence stopping at `Validate` would be
unreachable from the business logic that acts on it. The value reader (#113) is
the third state presence cannot express: `ok` is a carried value, `!ok` with
`Has<Field>()` is an explicit null `Apply` will Clear, `!ok` without it is
absent. Its body is a deliberate mirror of `Apply` and is uniform across every
field shape — every patch field is `*T` — and it uses `var zero {{ $f.Type }}`
because the type may be a slice or a map. Only the wrapper gets one: the raw
request already exports its `*T` fields. These rules are load-bearing:

- **Field shape comes from ent, never from a second opinion.** `funcs_presence.go`
  is the whole rule set: a create field is `*T` when `Optional || Default ||
  Nillable` — exactly when ent can fill it without the caller — and required when
  `!Optional && !Default`. `Hidden` and `ReadOnly` remove request fields; ent
  decides which setters exist, so any independently derived shape shows up as a
  call to a method that was never generated.
- **`Apply` uses `if r.X != nil { b.Set<X>(*r.X) }`, never `SetNillable<X>`.**
  ent skips the nillable setter for a field whose type is already nillable, so
  `SetNillableTags` does not exist for an optional `field.JSON`. The spike's
  hand-written target uses `SetNillable` because its schema has no JSON field;
  one uniform branch is correct for every shape and is what ent's own
  `SetNillable` expands to anyway.
- **Presence means different things in the two requests, on purpose.** A create
  request cannot express "clear", so a JSON `null` is recorded as absent and the
  field goes unwritten — which is also what makes an explicit null on a required
  field a "required" error. A patch records `null` as present, because that is
  how clearing is expressed.
- **A request that never went through `UnmarshalJSON` defaults in opposite
  directions.** `has()` on a create request returns true for everything (the
  struct is the only source of truth there); on a patch request it returns false
  for everything (nil pointers must never read as "clear the row"). The
  unmarshaller always allocates the map, so neither fallback fires on a decoded
  request.
- **Requiredness is checked by presence, not by the zero value**, except for
  strings, where `== ""` says the same thing and matches the spike. `0` and
  `false` are values; #14 exists because the v1 template only checked strings.
- **`patchFields` starts from ent's `MutableFields`, then removes `Hidden` and
  `ReadOnly`.** A surviving field provably has a `Set<Field>`. It is clearable
  exactly when Ent marks it `Optional`.
- **The value readers put a field-derived name in a method set, which is new,
  so two field names are now refused** (`patchMethodCollisions`): a
  patch-visible field whose Go name is `Apply`, and a patch-visible pair `x` /
  `has_x`. The second one predates the readers — #98 put `Has<Field>()` on the
  raw request, where `has_x` is a struct field of that very name — and it is
  gated on patch visibility ONLY. `Except(api.OpPatch)` is deliberately not a
  gate: `dto.tmpl` renders the patch request, its wrapper, `Apply` and the
  readers unconditionally, which `internal/fixtures/wiring/wiringent/patchless_dto.go`
  shows.

An `Immutable()` field is absent from the PATCH DTO. The generated HTTP handler
closes the resulting silent-drop case by comparing raw keys against the
generated patch-tag data before calling the DTO's custom unmarshaller; à la
carte DTO decoding remains permissive for unrelated keys.

### Response, summary and edge generation (#25)

`dto.tmpl` emits, per entity: `{Entity}Response`, `{Entity}Summary`,
`New{Entity}Response` (returns an error), `New{Entity}Summary` (cannot),
and `{Entity}QueryWithResponseEdges`. Four rules are load-bearing, all
established against real ent and real SQLite in #22 — do not re-derive them
from the templates:

- **Edges are selected by their own `api.Expand()` annotation**, never from
  foreign-key placement. Deriving it from the FK made a to-many edge
  permanently unreachable (`edge.Field()` is nil when the column is on the
  other entity) and fused "expose `author_id`" with "expose the nested
  `author`". `responseEdges` (`funcs_fields.go`) is the template entry point;
  `responseEdgeSet` (`funcs_fields.go`) is the pure selector.
- **`responseEdges` returns an error** when an expanded edge targets a
  non-Resource entity. The generator skips such a node, so no
  `<Target>Summary` exists; dropping the edge would silently narrow the
  response, and emitting the reference would surface as an undefined symbol in
  the consumer's build.
- **Edge state goes through `<Edge>OrErr()`, never a nil check.** `loadedTypes`
  is unexported. Loaded-and-absent is an explicit `null` (no edge field is
  `omitempty`); not-loaded is an error. `IsNotFound` in `dto.tmpl` is
  unqualified so it binds to Ent's generated predicate in the consumer's
  `package ent`, and `TestDTOTemplateResolvesIsNotFoundToEnt` pins it.
- **Summaries carry no edges.** That is what bounds expansion — there is no
  second level for a cycle to close through, so no runtime depth counter and no
  visited set. A three-level tree comes back one level deep; that cost is
  asserted, not glossed.

Because the response and summary structs are emitted for **every** entity, not
only those with a response-visible field, `dtoImports` adds `node.ID`'s import
unconditionally (`funcs_imports.go`). Gating it on a non-empty `responseFields`
left an all-sensitive entity's ID import undeclared, and
`TestTemplatesDeclareTheirImports` caught it only once the `edges` fixture —
which has such an entity — was added to that test's corpus. Edges themselves
need no import: `<Target>Summary` is package-local.

`{Entity}EntToResponse` is gone (#29). It delegated to `New{Entity}Response` and
**returned nil on the error**, so a not-loaded edge reached the caller as a nil
pointer instead of an error naming it. That is the correctness argument for its
removal, not merely a redundancy one.

**Open, and deliberately not decided here:** which scalar fields a summary
carries. Nothing in the schema says "this field is the brief one", so the
generator does not guess — a summary carries every response-visible field, minus
the edges. The spike's hand-written `UserSummary{ID, Name}` picked one field by
judgement, which is the single thing in `internal/fixture/spikeent/dto/` that is not
mechanical. Narrowing it needs a new annotation, and that is a separate issue.

## Annotation model

`api/annotations.go` defines exactly three annotations. `api.Resource()` is the
sole entity switch and `.Except(Op...)` subsets public operations. Fields are
silent by default: Optional/Default/Nillable/Immutable/Sensitive/type determine
shape, and exactly five deviation words remain — Hidden, ReadOnly, Searchable,
Filterable, Sortable. `api.Expand()` is the sole edge word; `.JSONKey()` changes
only its response key.

All builders use value receivers and return copies. All three annotation types
implement `schema.Merger`: field booleans union, Except operations union, and
JSONKey is last-non-empty-wins. This is load-bearing because Ent otherwise
silently overwrites separate same-name annotations during schema loading.

Every exported knob reaches a registered template function; `pendingKnobs` is
empty. The scope charter remains: annotations control HTTP generation and do not
remove service-layer wiring or request DTOs. Only a provably unusable create
family may disappear when `OpCreate` is explicitly excepted.

## Conventions baked into generated code

- **No identifier type is hardcoded anywhere.** Every template renders the id
  through `$.ID.Type` and asks `wiringImports`/`dtoImports` for its package, so
  an `int` key needs no import at all (the `intid` fixture covers exactly that).
  The `uuid.UUID`-only refusal in `schema_conflicts.go` belonged to the base
  service and base handler templates and went with them (#29).
- `Create{Entity}`/`Patch{Entity}` take the **validated** request. They have no
  choice: `Apply` exists only on `Valid{Entity}…Request`. The exported
  `Apply{Entity}CreateRequest`/`Apply{Entity}UpdateRequest` free functions are
  gone, because taking a raw request is exactly the escape hatch that made
  validation optional (#26).
- **Soft delete is annotation-based and lives at ent's layer** (#18, #70). A
  consumer only embeds `entapi.SoftDeleteMixin` (field + `DomainSoftDelete`
  marker; ent merges mixin annotations onto the type). The
  `config/init/fields/*` partial writes the hook and interceptor directly into
  each soft-deletable type's fresh `cfg.hooks` / `cfg.inters` slices inside
  `newConfig`, before `cfg.options(opts...)`; `NewClient`, `Open`, `enttest.Open`
  and every later config copy therefore carry them with no registration call
  or initialization-order dependency. Injection makes the soft-delete hook
  `hooks[0]`; Ent applies index 0 outermost, ahead of later consumer `Use`
  hooks. Nothing in the generated wiring knows about it: `Delete{Entity}`
  issues `DeleteOneID(...).Exec` (`OpDeleteOne`) and `DeleteBatch{Entities}`
  issues `Delete().Where(IDIn(...)).Exec` (`OpDelete`), and the hook rewrites
  both. There is no second tombstone write. The old
  `deleted_at`/`Nillable` naming convention is gone: #18 retired it (it could not
  tell an entity that opted in from one that merely owns a column with that
  name, and it only ever reached the write path), and #29 removed the last
  caller of `hasSoftDelete`/`isTimeField` along with `base_service.tmpl`.
  **The traverser filters eager loads too**, not only top-level queries: a
  `With<Edge>()` sub-query runs on the target's own builder. **Soft delete does
  not cascade** — a row owning a soft-deleted target keeps its foreign key and
  stays in every list, so a `Required()` + `api.Expand()` edge surfaces as JSON
  `null` (#100); `internal/softdeleteproof` proves both against real SQLite.
- **Five graph-level templates** are rendered once over `*gen.Graph` rather than
  per `*gen.Type`; their output files are cleaned up by the same marker scan as
  everything else (#63), with no per-name enumeration:
  `templates/softdelete.tmpl` -> `entapi_softdelete.go`, generated for a
  soft-deletable entity even when that entity is not an API Resource; and
  `templates/errors.tmpl` -> `entapi_errors.go`, generated when any entity is
  annotated, holding the package's `ErrorMap` (#13); and
  `templates/http.tmpl` -> `entapi_http.go`, holding `APIHandler`, `API(client)`,
  `ServeHTTP`, `Mount`, the endpoint manifest and request-time function fields —
  plus one named `…Endpoint()` accessor per reachable operation (the wiring
  function's name plus `Endpoint`, so list is plural: `GetArticleEndpoint()`,
  `ListArticlesEndpoint()`) and
  `OpenAPIEndpoint()` for the manifest's Op-less entry (#119). The manifest is
  **built from those accessors**, so take-one-by-name and the batch loop share
  one construction and cannot describe different endpoints; the accessors are
  methods on `APIHandler`, so they add no new collision shape — `Except` removes
  the method with the endpoint, which is what makes naming a removed operation a
  compile error; and
  `templates/openapi.tmpl` -> `openapi.yaml` with
  `templates/openapi_embed.tmpl` -> `entapi_openapi.go`, gated together on the
  same `wiredAny` condition as the route tree they describe.
- **Every exported wiring function returns through `ErrorMap.MapError`, exactly
  once** (#13). The unexported `{entity}Get` exists precisely so a create or
  update that re-reads through the eager-load plan does not map twice. The two
  base predicates, `IsValidationError`, and `ValidationError` in `errors.tmpl`
  must stay UNQUALIFIED — runtime predicates with similar names would still
  compile while silently classifying nothing. `MapError` is ordered nil →
  not-found → validation → constraint-and-unique → passthrough. `API(client)`
  installs `UniqueViolation(client.driver.Dialect())` only when
  `HasUniqueViolation()` is false, preserving a consumer determination installed
  before API construction. PostgreSQL SQLSTATE is authoritative when present;
  MySQL and SQLite use dialect-gated text markers, and every miss fails closed
  to 500 rather than inventing a 404/409/422 sentinel.
- **HTTP handlers are bind → call → write.** Their middle step reads the
  unexported function field through `h` at request time; capturing a default
  wiring function while `API()` builds the route would create a dead
  customization point for #75. There is no customization point inside a handler body and
  no generated hook/interceptor chain; cross-cutting concerns wrap the returned
  `http.Handler`.
- **The bind step's two hardenings have no knobs, and that is the design.**
  Every POST/PATCH handler binds through `entapi.BindJSON`, which opens with
  `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` → 413 and an
  `application/json` media-type check → 415 (`runtime/bind.go`; #103 moved both
  out of `templates/handler.tmpl`, unchanged). Neither is configurable: `With` accepts exactly one
  family — per-operation replacement functions whose signature is the wiring
  function's — and a global config knob would make it two families, which is
  where its compile-time guarantee comes from. 1 MiB is therefore a hard ceiling
  with no way out in-band; wrapping a larger `MaxBytesReader` outside does not
  help, since the inner smaller limit still wins.
- **Wiring and customization points take `*Client`, and this package never
  generates a transaction boundary.** No `*Tx` variant, no
  transaction-from-context: a `*Tx` twin would break the property that a
  customization point's signature is character-for-character the wiring
  function's, which is what turns a wrong replacement into a compile error.
  Composing with an outer transaction is ent's own `tx.Client()` (its `config`
  carries the `txDriver`). `API(client)` holds the root client, so the `db` a
  custom implementation receives on the HTTP path is NOT transaction-bound.
- `With` mutates `APIHandler` with whole-operation custom implementations; finish wiring before serving, because later calls race with request-time field reads.
- `Endpoints()` returns a fresh copy in registration order, while `Mount` and the internal mux walk the single unexported endpoint list.
- Handler code should not import `ent` for conversion. That is a property of
  where the DTO package sits, not of a base type — `Base{Entity}Handler`
  required an `ent` import to embed it, so it never achieved the goal. Today the
  DTOs land in the consumer's `package ent` and the free functions are what a
  handler calls; the dependency a handler genuinely cannot avoid is
  `entapi/runtime`, which is stdlib-only.

## Generation can fail, and that is a feature (#10)

`generatePerTypeFiles` calls `checkGraphConflicts` (`schema_conflicts.go`)
**before** `next.Generate(g)`, so a rejected schema leaves nothing on disk —
not even ent's own output. The policy it implements, which later generation
slices are expected to follow:

> An annotation that contradicts the ent schema fails generation, reporting
> both facts. Anything that can be generated correctly is generated, not
> refused.

The matrix rejects contradictory deviation words, type-incompatible query
dimensions, secrets exposed as query or read-only fields, create requests that
cannot satisfy required-no-default Ent fields, required edges that declare no
`edge.Field(...)` while the create family is reachable, empty public PATCH surfaces,
misplaced words, words on IDs, query words with `OpList` excepted, `_`-prefixed
query storage keys, expansion to non-resources, asymmetric self edges, invalid
soft-delete declarations, generated-name collisions, and the two patch-visible
field names that collide with a method the patch DTO generates on the same
receiver. HTTP rows skip nodes
without `api.Resource()`; soft-delete and symbol checks remain graph-wide.

Conversely, `Optional().Nillable()` and named `GoType`s over slices/maps *are*
generated, because correct output exists for them — `*T` in the create request
(`dto.tmpl`), and `PtrNilSafe` chosen by `isComplexFieldType`, which reads
`field.Type.RType.Kind` rather than the rendered type name. The full table is
in README.md, "Field shapes".

Every row has a fixture. A fixture whose generation must fail carries
`wantGenErr` in the `fixtures` table and has no generated output to commit; see
`internal/fixtures/README.md`.

## Testing conventions

- Tests are in-package (`package entapi`) in both packages. Generator tests build `gen.Field`/`gen.Type` values by hand via the constructors in `test_helpers_test.go` (`newStringField`, `newUUIDField`, `newTestType`, `ptr`, `assertContains`, …). Use those instead of hand-rolling literals. A test for a runtime symbol belongs in `runtime/`, and must not pull ent in — that is what keeps the runtime testable without the generator's dependency graph.
- `funcs_codegen.go` helpers are tested by asserting on **substrings of emitted Go source**, not by compiling it.
- Two tests do render templates end-to-end: `TestCodegenFixtures` (generates into `internal/fixtures/<dir>/<dir>ent` and compiles the result) and `TestTemplatesDeclareTheirImports` (renders each template and checks goimports changes no import). A template edit that breaks compilation or import declarations is caught here; anything beyond that still needs a real ent project.
- **The fixture harness contract: one directory plus one line.** Add `internal/fixtures/<dir>/<dir>ent/schema/` with a hand-written ent schema, and one `{dir: "<dir>"}` line in the `fixtures` table in `codegen_fixtures_test.go`. Nothing else in the harness changes. A schema the generator must **refuse** is the same directory plus `wantGenErr: []string{…}`: generation is then required to fail, its error must contain every listed substring, and nothing may be written under `<dir>/<dir>ent` besides the hand-written schema. "It failed" is not the assertion — the message is, because the message is all a schema author gets.
- **The generated directory is `<dir>ent`, never `ent` (#49).** A Go package is named after the last element of its import path, so `internal/fixtures/basic/basicent` declares `package basicent`. That is deliberate and load-bearing: goimports resolves a bare `ent.` reference by **package name**, and when thirteen fixtures all declared `package ent` the winner depended on its module index cache — which is how one run rewrote `entgo.io/ent` in `softdelete`'s `client.go` to `basic`'s package. The harness derives every path from `entDirName`/`fixtureEntPkgPath` in `codegen_fixtures_test.go`, and `TestNoAmbiguousEntPackages` fails the run if any package under `internal/fixtures` is named `ent`.
- `TestCodegenFixtures` **writes into the repository tree** on purpose: generated ent code has to sit inside this module to resolve `github.com/githonllc/entapi` without a replace directive, and `t.TempDir()` is outside any module. The output is committed, so a clean checkout plus a test run leaves `git status` clean. **A dirty tree after `make check` means generation changed** — regenerate, never hand-edit anything under `internal/fixtures/<dir>/<dir>ent/`.
- **`internal/fixtures` (plural) is the generator's output; `internal/fixture` (singular) is its target.** The singular one is a separate Go module holding the #22 spike: `ent/dto/` there is hand-written, compiled and exercised against SQLite, and it is the specification. Read it; never edit it — except for a mechanical migration every consumer also has to make, which is how #15 changed exactly one import line in six of its files.
- **Five nested modules run under `make test-modules`, and each exists because a compile proof cannot answer its question.** They are separate modules because they need a SQL driver and this library must not have one.
  - `internal/fixture` — the #22 spike. It breaking is the signal that generated output has drifted from the hand-written target.
  - `internal/fixtures/wiring/e2e` — the behavioural half of the wiring fixture. Compiling proves the wiring type-checks; only this proves it returns the right page, and it carries #13's three-way missing-row / `UNIQUE` / `FOREIGN KEY` mapping proof against real SQLite.
  - `internal/fixtures/httpdemo/e2e` — the HTTP tracer bullet: all five endpoints, problem+json statuses, direct/Mount/StripPrefix composition, actor context, middleware short-circuiting and Excepted routes against real SQLite. It is also where #76's document is validated against the OpenAPI 3.1 meta-schema (`pb33f/libopenapi` + `libopenapi-validator`, which live here and nowhere else — the root module must stay free of a YAML parser) and where it is checked against reality three ways: schemas versus the generated structs by reflection, documented paths versus `Endpoints()`, and each field's documented operator prefixes versus what the live parser accepts. Any one of the three alone proves only that the file parses.
  - `internal/softdeleteproof` — the only evidence for #18's load-bearing claim: a direct `client.Doc.Query()`, with nothing generated in the call path, does not return deleted rows. A compile proof cannot tell "the predicate is generated" from "the predicate reaches the SQL".
  - `internal/uniqueproof` — #74's dialect determinations against the **real** driver error types. It exists because a hand-written stand-in would prove only that the detector matches the stand-in: it takes `jackc/pgx/v5` (a genuine `*pgconn.PgError`, so the `SQLState()` probe is exercised, `23503` included as the negative) and `go-sql-driver/mysql` (a genuine `*mysql.MySQLError`, which is the negative case for the probe — `Number` and `SQLState` are fields, not methods — and the positive case for the `Error 1062` text). Those two drivers live here and nowhere else; the root module must stay free of them.
- `internal/fixtures/edges/edgesent/orerr_contract_test.go` is one of the few hand-written files under a generated `<dir>ent/` directory. It must be in `package edgesent` because it sets the unexported `Edges.loadedTypes`, which is the only way to construct *loaded and absent* without a database. The others include `basic/basicent/listresponse_shape_test.go`, `createexcepted/createexceptedent/create_family_test.go`, the two `httpdemo/httpdemoent/*_test.go` contract proofs, `presence/presenceent/account_presence_test.go`,
`fieldshapes/fieldshapesent/jsonwidget_reader_test.go`, `query/queryent/filter_contract_test.go`, `strictquery/strictqueryent/strictquery_filter_contract_test.go`, `sensitive/sensitiveent/sensitive_wire_test.go` and `stale/staleent/trinket_dto.go`; `go build` ignores `_test.go` files, so only the last of those is visible to the codegen harness — and it is there precisely to prove cleanup leaves files it did not write alone.
- The three nested modules that import a fixture package (`internal/fixtures/wiring/e2e`, `internal/fixtures/httpdemo/e2e`, `internal/softdeleteproof`) alias it back to `ent` in their imports. They are hand-written and read from the consumer's side, where the generated package really is named `ent`; the alias is spelled out, so goimports has nothing to resolve.

## Dead code is now a test failure, not a convention (#7)

The dead template, the dead registry entries, the unreachable field selectors,
and the test-local copy of the annotation decoder are gone. Three assertions
keep them from coming back — read them before adding to the registry:

- `TestTemplateInvocationsAreRegistered` (`template_funcs_consistency_test.go`)
  fails in **both** directions: a template calling an unsupplied function, and a
  registered entry no template invokes. The lists are derived from the parsed
  template trees, never hardcoded. Registering a helper "for later" fails CI.
- `TestTemplateFuncsDoNotShadowEntBuiltins` keeps `templateFuncs()` disjoint
  from `gen.Funcs`. `templateFuncMap()` overlays ours on Ent's, so a same-named
  entry would silently replace an Ent builtin. Use Ent's `lower`, `hasPrefix`,
  `camel`, `snake`, … directly; do not re-register them.
- `TestEveryEmbeddedTemplateIsLoaded` (`template_loader_test.go`) requires every
  embedded `.tmpl` to be bound in `template_index.go`, so a template nothing
  loads cannot survive unnoticed the way `model.tmpl` did.

A fourth guards the package boundary itself: `TestRuntimePackageIsGeneratorFree`
(`runtime_isolation_test.go`) shells out to `go list -deps -json` and requires the
runtime's transitive closure to contain no `entgo.io/ent/entc*`, no
`golang.org/x/tools/imports`, no `embed` and not the generator package. It is
paired with a **control** that points the same probe at the generator and
requires it to find `embed` with a non-empty `EmbedFiles` — so a probe broken by
a typo'd path or a failed `go list` fails loudly instead of passing as a vacuous
absence check. `runtimePackage` is bound to `defaultEntAPIPackage`, so it
cannot drift from what generated code actually imports. Write new absence
assertions the same way: an absence check without a positive control is a rubber
stamp waiting to happen. `runtime/runtime_deps_test.go` is the cheap parser-level
half, reading the package directory rather than a hand-maintained file list.

A fifth, one level up: `TestRemovedTemplatesStayRemoved` (`template_loader_test.go`)
pins that `base_service.tmpl` and `base_handler.tmpl` are gone **and** that a
marker-bearing `widget_base_service.go` / `widget_base_handler.go` still gets
deleted from a target directory. Cleanup is the only thing that removes them
from a consumer's tree, so the *behaviour* has to outlive the template — the
test pins that behaviour rather than the mechanism, which is why replacing name
enumeration with a marker scan (#63) did not need it weakened.

Related: `templates/dto.tmpl` calls `IsNotFound` **unqualified on purpose** —
the emitted file lands in the consumer's `package ent`, so it binds to Ent's
generated predicate, not to the runtime's `IsNotFound` (`runtime/errors.go`).
Qualifying it would compile and silently stop matching, routing every
loaded-but-absent edge into the error branch.
`TestDTOTemplateResolvesIsNotFoundToEnt` pins that direction, and
`TestTemplatesQualifyEntapiSentinels` pins the converse across every
template: `entapi.Err*` always stays qualified, because `package ent` has no
such symbol.

### Baseline state

`make check` (including `test-modules`), `go test ./...`, `gofmt -l .` and `make lint` are all green on a clean checkout, and `git status --porcelain` is empty after `make check`. **Anything you see is yours.**

`golangci-lint` and `goimports` are not on the default PATH; run lint as `PATH=$PATH:$HOME/go/bin make lint`. It must be **golangci-lint v1** — v2 rejects this repo's `.golangci.yml` schema.

`gofmt -l .` covers the whole tree, but `make fmt` deliberately does not: `FMT_FILES` excludes `internal/fixture/` and `internal/fixtures/`, because gofmt walks the filesystem rather than the module graph and would otherwise rewrite generated files that the next test run rewrites back. Hand-written files under those prefixes — the fixture schemas, and the hand-written `_test.go` files inside generated `ent/` directories — must be kept gofmt-clean by hand.

## Docs to keep in sync

`README.md` and `README_zh.md` are parallel translations; changing the public API means editing both, and both carry the migration notes for every symbol this module has removed. There are now **two** `doc.go`s: the root one is the generator's godoc quick start and the runtime migration pointer, `runtime/doc.go` is the runtime's. `docs/ARCHITECTURE.md` carries the module table and the PlantUML diagrams, and `.claude/skills/entapi/SKILL.md` documents downstream usage patterns (some of it describes a consumer project's interceptors, not this repo).

**`docs/DESIGN-v2.md` and `docs/DESIGN-v3-final.md` are plans, not references.** Both still say implementation has not started; both are stale in that claim, and v3's eight slices (#69–#76) have all landed, closed by #77, #78, #81, #82, #84, #85, #86 and #87. Read them for decisions and rationale, never for the current API — three v3 items were superseded during implementation (`*APIHandler` not `*API`; soft delete installs from a `config/init/fields/*` partial with no `init()` and no `RegisterSoftDelete` fallback; unknown request keys are caught by comparing raw keys against generated `{entity}{Op}RequestTags` rather than by `DisallowUnknownFields`). The divergence tables live in both READMEs, under "Deviations from DESIGN-v3".

Moving or removing a published symbol is an established move here, not an exception — #3, #6, #24, #26, #29 and #15 all did it. The convention is a migration note in **both** READMEs plus a `doc.go` pointer, and no compatibility alias: an alias that preserves the coupling a change exists to remove is worse than the break.
