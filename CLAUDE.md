# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make check                  # fmt + vet + test + test-modules (run before committing)
make test                   # go test -count=1 -v ./...  (this module only)
make test-modules           # the three nested modules; NOT covered by make test
make cover                  # coverage summary (CONTRIBUTING targets >85%)
make lint                   # golangci-lint run ./...   (v1; v2 rejects .golangci.yml)
make fmt                    # gofmt + goimports -local github.com/githonllc/entdomain

go test -run TestCamelCase -v ./.          # single test, generator package
go test -run 'TestErrorMapper_.*' ./runtime  # subset by regex, runtime package
```

Note: the Makefile overrides `GOPATH=/tmp/gopath` and `GOMODCACHE=/tmp/gomodcache`. Bare `go test ./...` uses your normal module cache and is equivalent otherwise.

`make test` does **not** reach the nested modules — they are separate `go.mod`s, so `./...` never descends into them. `make check` runs both. A change that compiles and passes `make test` can still be caught by `make test-modules`; that is the point of it.

## What this repository is

One Go module, **two packages**, split by *when the code runs* (#15). Both are `package entdomain`, so every call site reads `entdomain.X` whichever one it came from; only the import path differs.

1. **`github.com/githonllc/entdomain` — code-generation time.** An [Ent](https://entgo.io) extension. Consumers wire it into their `entc.go`; it reads `DomainField` annotations off `gen.Field`s and writes `{entity}_dto.go`, `{entity}_filter.go`, `{entity}_wiring.go` per annotated entity, plus `entdomain_errors.go` and `entdomain_softdelete.go` per graph, into the consumer's `ent/` package. Schema files import this one too, for the annotation builders, `Edge()` and `SoftDeleteMixin`.
2. **`github.com/githonllc/entdomain/runtime` — application run time.** The types generated code links against: `Page`/`ListRequest`, the generic `ListPage`/`GetOne`/`SaveOne`, `AppendIf`/`AppendIfSlice`, `ErrNotFound`/`ErrAlreadyExists`/`ErrValidation`, `ErrorMapper`, `Ptr`/`PtrOrNil`/`PtrNilSafe`, and the soft-delete context switches. There is no cursor type: `Cursor`, `PageInfo`, `EncodeCursor`, `DecodeCursor` and `ListRequest.Cursor` were removed on #6, and pagination is offset-only.

**The split is load-bearing, not cosmetic.** `template_index.go` declares package-level vars calling `mustLoadTemplate`, so while the two halves shared a package, a consumer's production binary that wanted `ErrValidation` embedded five templates and ran the template loader at init. Measured, and asserted by `TestRuntimePackageIsGeneratorFree`: `go list -deps ./runtime` reports **0** `entgo.io` packages out of 62 (all standard library) and `EmbedFiles` is empty, against **15** for the generator — which is correct there, since generating genuinely needs ent.

Keep new code on the right side of that line. Anything the generated output calls at run time belongs in `runtime/` and must import stdlib only; anything ent loads belongs in the root. `softdelete.go` is the worked example of the seam: the mixin (schema-time, imports `entgo.io/ent/schema`) stayed, while `WithSoftDeleted`/`WithHardDelete` and their predicates moved to `runtime/softdelete_context.go` because the generated traverser and hook call them on every query and every delete.

There is no `main`, no example app, and no downstream ent project in this repo. Anything about *generated* code can only be verified by generating into a real ent project — which `TestCodegenFixtures` and the three nested modules do.

## Generation pipeline

`extension.go` is the whole entry point:

- `Hooks()` returns one `gen.Hook` (`generatePerTypeFiles`). **`Templates()` deliberately returns an empty slice** — this extension does not use Ent's `GraphTemplate` mechanism. Do not "fix" that by adding templates there.
- The hook runs *after* `next.Generate(g)`, then loops `g.Nodes`. **A node with zero `domainFields` is skipped entirely** — that's how unannotated entities avoid producing empty files. Files an earlier run wrote for a node that is now skipped are deleted afterwards; see "Stale artifact removal" below.
- Each file is rendered with `text/template`, then run through `golang.org/x/tools/imports` in `writeFile`. **Formatting failure aborts generation and writes nothing** — `imports.Process` only fails on source it cannot parse, so its failure is a template bug. The write itself goes to a temp file in the target directory and is renamed into place, so a run that fails partway leaves the previous output intact.
- Templates declare the imports their output uses (`dtoImports` in `funcs_imports.go` computes the field-type ones). goimports stays in the pipeline as a safety net, not as the mechanism — it cannot be the mechanism now that its failure is fatal. `TestTemplatesDeclareTheirImports` fails if goimports has to add or remove anything.
- `templateFuncMap()` layers three sources, later wins: Ent's `gen.Funcs` → this package's `templateFuncs()` → `entdomainPkg` (closure over the configured import path).

The full order inside the hook is: `checkGraphConflicts` (`schema_conflicts.go`, refuses contradicting annotations *before* ent writes anything) → `next.Generate(g)` → the per-node loop → stale artifact removal.

### Stale artifact removal

`cleanup.go` runs after every file of a successful run is on disk and deletes what this extension wrote earlier but did not write this time. It deletes files from the consumer's repository, so all three fences have to hold:

1. only file names this extension can produce, for entities present in the schema this run examined — `generatedFileNames(node)` for per-type files and `graphFileNames()` for the two graph-level ones. That per-type list is **five** names, not three: `_base_service.go` and `_base_handler.go` are names the generator can no longer write and must still delete, for a consumer upgrading past #29. Dropping them turns removal into a collision;
2. only files whose **first line** carries `Code generated by entdomain extension` — deliberately narrower than ent's own `Code generated by ent, DO NOT EDIT.`;
3. never a path this run just wrote.

Candidates that fail fence 2 are left alone and logged. Cleanup never runs on a failed generation, so neither a template bug nor a refused schema can take the previous output with it.

### Templates

Five live templates, all embedded and all bound: `dto`, `filter`, `wiring` (rendered per `*gen.Type`) and `errors`, `softdelete` (rendered once per `*gen.Graph`). `TestEveryEmbeddedTemplateIsLoaded` requires every embedded `.tmpl` to be bound in `template_index.go`, so there is no such thing as an embedded-but-unused template here.

`templates/*.tmpl` are embedded via `//go:embed` (`template_loader.go`) and loaded into package-level vars by `template_index.go` using `mustLoadTemplate`, i.e. **at package init — renaming or deleting a template panics on import**, not at generation time. That init is exactly what `runtime/` exists to stay out of; keep the embed directive and `templates/` in the generator package.

Per-type templates receive a `*gen.Type` as `.`, so `$.Config.Package`, `$.Package`, `$.Name`, `$.ID` and the standard Ent template funcs are all available. The two graph-level ones receive a `*gen.Graph`, so they see `$.Nodes` instead.

**Every template imports the runtime under an explicit `entdomain` alias**, and the alias is required rather than decorative: the path's last element is `runtime`, so goimports would read an unaliased import as package `runtime`, find no use of that name, and delete it — and `writeFile` aborts on a formatter failure.

### Template functions

Split by concern across `funcs_*.go`, and registered in one map in `funcs.go`:

| File | Contains |
|---|---|
| `funcs_fields.go` | field selection: `domainFields`, `createFields`, `patchFields`, `responseFields`, `responseEdges` |
| `funcs_scope.go` | `hasDomainScope`, `isDomainRequired`, and `getDomainFieldAnnotation` |
| `funcs_presence.go` | request field shape: `isCreatePointer`, `isCreateRequired`, `isPatchClearable` |
| `funcs_typechecks.go` | `isComplexFieldType` — **not registered as a template func**; only `fieldValueExpr` calls it |
| `funcs_softdelete.go` | `isSoftDeletable`, `softDeleteTypes`, `softDeleteField`, `softDeleteImports` — the soft-delete mixin's marker |
| `funcs_imports.go` | `dtoImports`: the import specs the DTO must declare for its field types |
| `funcs_codegen.go` | `fieldValueExpr` — the response constructor's per-field expression, and the only caller of `isComplexFieldType` |
| `funcs_strings.go` | `camelCase`, and nothing else — Ent's `gen.Funcs` already supplies `contains`, `hasPrefix`, `lower`, `snake`, `plural`, … |
| `funcs_filter.go` | the query surface: `queryFields` (the `ScopeQuery` gate), `isFilterable`/`isSearchable`/`isSortable` (the per-dimension markers), `searchFields`, `filterParams`, `filterImports` |
| `annotations_edge.go` | edge annotation: `DomainEdge`, `getDomainEdgeAnnotation`, `responseEdgeSet`, `edgeJSONKey` |

**A helper is only callable from a template if it appears in `templateFuncs()`.** Adding a func to a `funcs_*.go` file is not enough.

### Annotation access

Never read `field.Annotations["DomainField"]` directly. Always go through `getDomainFieldAnnotation` (`funcs_scope.go`): the annotation arrives as `*DomainField` during codegen but as `map[string]interface{}` when loaded from a serialized schema, and that function normalizes both via a JSON round-trip.

### Create and patch requests (#26)

`dto.tmpl` emits, per entity: `{Entity}CreateRequest`, `{Entity}PatchRequest`,
a `Valid…` wrapper for each, an `UnmarshalJSON`, one `Has<Field>()` per field,
and `Apply` on the validated types only. Five rules are load-bearing:

- **Field shape comes from ent, never from a second opinion.** `funcs_presence.go`
  is the whole rule set: a create field is `*T` when `Optional || Default ||
  Nillable` — exactly when ent can fill it without the caller — and required when
  ent requires it and cannot default it, or when the annotation says so. ent
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
- **`patchFields` intersects ScopeUpdate with ent's `MutableFields`.** That is
  the list ent's setter template iterates, so a field that survives provably has
  a `Set<Field>`. `checkGraphConflicts` refuses Immutable+ScopeUpdate first, so
  the intersection currently drops nothing — the refusal is what the author
  sees, the filter is what keeps the output correct.

The one case that cannot be closed here: an `Immutable()` field named in a PATCH
body is discarded by `encoding/json` before any validator runs. Rejecting it
needs `DisallowUnknownFields` in the consumer's handler.

### Response, summary and edge generation (#25)

`dto.tmpl` emits, per entity: `{Entity}Response`, `{Entity}Summary`,
`New{Entity}Response` (returns an error), `New{Entity}Summary` (cannot),
and `{Entity}QueryWithResponseEdges`. Four rules are load-bearing, all
established against real ent and real SQLite in #22 — do not re-derive them
from the templates:

- **Edges are selected by their own `DomainEdge` annotation**, never from
  foreign-key placement. Deriving it from the FK made a to-many edge
  permanently unreachable (`edge.Field()` is nil when the column is on the
  other entity) and fused "expose `author_id`" with "expose the nested
  `author`". `responseEdges` (`funcs_fields.go`) is the template entry point;
  `responseEdgeSet` (`annotations_edge.go`) is the pure selector.
- **`responseEdges` returns an error** when a response-scoped edge targets an
  entity with no `DomainField` at all. The generator skips such a node, so no
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
only those with a response-scoped field, `dtoImports` adds `node.ID`'s import
unconditionally (`funcs_imports.go`). Gating it on a non-empty `responseFields`
left an all-`InputOnly` entity's ID import undeclared, and
`TestTemplatesDeclareTheirImports` caught it only once the `edges` fixture —
which has such an entity — was added to that test's corpus. Edges themselves
need no import: `<Target>Summary` is package-local.

`{Entity}EntToResponse` is gone (#29). It delegated to `New{Entity}Response` and
**returned nil on the error**, so a not-loaded edge reached the caller as a nil
pointer instead of an error naming it. That is the correctness argument for its
removal, not merely a redundancy one.

**Open, and deliberately not decided here:** which scalar fields a summary
carries. Nothing in the schema says "this field is the brief one", so the
generator does not guess — a summary carries every response-scoped field, minus
the edges. The spike's hand-written `UserSummary{ID, Name}` picked one field by
judgement, which is the single thing in `internal/fixture/ent/dto/` that is not
mechanical. Narrowing it needs a new annotation, and that is a separate issue.

## Annotation model

`annotations.go` defines `DomainField` plus value-receiver fluent builders (`WithRequired`, `AsSearchable`, `WithFormat`, …). Every builder **returns a copy** — chaining works, mutating in place does not.

**No preset builder grants `Searchable`, `Filterable` or `Sortable`.** The three markers are opt-in per field (#27): they now generate real query parameters and a real sort allow-list, and a permissive default would make essentially every response-visible field orderable. Presets do still grant `ScopeQuery` — that scope says the field *may* be reached from the query API, and the marker says in which dimension. A marker without `ScopeQuery` is refused at generation time, as is `Searchable` on a type with no `Contains`, `Filterable` on a type with no ops, and `Sortable` on a non-comparable type (`schema_conflicts.go`).

**Seven of the 27 exported knobs reach a template**: `DomainField.Scopes`, `DomainField.Required`, `DomainEdge.Scopes`, `DomainEdge.JSONKey` and three of the four scopes. The other 20 are accepted, stored and ignored. That is allowed but not free: `TestEveryAnnotationKnobIsConsumedOrDeclaredPending` (`annotation_surface_test.go`) derives the knob list by reflection and reachability by toggling each knob against the registered template funcs, so a new knob fails CI until it is either wired up or given a `pendingKnobs` entry naming its issue. It also fails when a listed knob *becomes* reachable — the entry is a claim with a deadline, not an exemption. See "Dead code is now a test failure" below; this is the same contract one level up.

Preset builders (`DefaultField`, `InputOnlyField`, `OutputOnlyField`, `CreateOnlyField`, `IdField`, `AuditLogField`) are just scope combinations layered on `DomainFieldWithScopes`.

The load-bearing design rule, repeated throughout the code and README: **scopes only control HTTP-layer struct generation.** They never restrict what the service layer can do with an ent entity. Keep new features on that side of the line.

## Conventions baked into generated code

- **No identifier type is hardcoded anywhere.** Every template renders the id
  through `$.ID.Type` and asks `wiringImports`/`dtoImports` for its package, so
  an `int` key needs no import at all (the `intid` fixture covers exactly that).
  The `uuid.UUID`-only refusal in `schema_conflicts.go` belonged to the base
  service and base handler templates and went with them (#29).
- `Create{Entity}`/`Update{Entity}` take the **validated** request. They have no
  choice: `Apply` exists only on `Valid{Entity}…Request`. The exported
  `Apply{Entity}CreateRequest`/`Apply{Entity}UpdateRequest` free functions are
  gone, because taking a raw request is exactly the escape hatch that made
  validation optional (#26).
- **Soft delete is annotation-based and lives at ent's layer** (#18). A consumer
  embeds `entdomain.SoftDeleteMixin` (field + `DomainSoftDelete` marker; ent
  merges mixin annotations onto the type) and calls the generated
  `ent.RegisterSoftDelete(client)` once. Nothing in the generated wiring knows
  about it: `Delete{Entity}` issues `DeleteOneID(...).Exec` (`OpDeleteOne`) and
  `DeleteBatch{Entities}` issues `Delete().Where(IDIn(...)).Exec` (`OpDelete`),
  and the hook rewrites both. There is no second tombstone write. The old
  `deleted_at`/`Nillable` naming convention is gone: #18 retired it (it could not
  tell an entity that opted in from one that merely owns a column with that
  name, and it only ever reached the write path), and #29 removed the last
  caller of `hasSoftDelete`/`isTimeField` along with `base_service.tmpl`.
- **Two graph-level templates** are rendered once over `*gen.Graph` rather than
  per `*gen.Type`, and both their output files are cleaned up through
  `graphFileNames()`, not `generatedFileNames(node)`:
  `templates/softdelete.tmpl` -> `entdomain_softdelete.go`, generated for a
  soft-deletable entity even when that entity has no domain fields at all; and
  `templates/errors.tmpl` -> `entdomain_errors.go`, generated when any entity is
  annotated, holding the package's `ErrorMap` (#13).
- **Every exported wiring function returns through `ErrorMap.MapError`, exactly
  once** (#13). The unexported `{entity}Get` exists precisely so a create or
  update that re-reads through the eager-load plan does not map twice. The two
  predicates in `errors.tmpl` must stay UNQUALIFIED — `entdomain.IsNotFound`
  also exists and would still compile, silently classifying nothing.
- **There are no generated hooks and nothing to embed.** The wiring is free
  functions; a consumer who needs different behaviour writes their own function
  and stops calling the generated one. `SetSelf` dispatch is gone (#16, #29).
- Handler code should not import `ent` for conversion. That is a property of
  where the DTO package sits, not of a base type — `Base{Entity}Handler`
  required an `ent` import to embed it, so it never achieved the goal. Today the
  DTOs land in the consumer's `package ent` and the free functions are what a
  handler calls; the dependency a handler genuinely cannot avoid is
  `entdomain/runtime`, which is stdlib-only.

## Generation can fail, and that is a feature (#10)

`generatePerTypeFiles` calls `checkGraphConflicts` (`schema_conflicts.go`)
**before** `next.Generate(g)`, so a rejected schema leaves nothing on disk —
not even ent's own output. The policy it implements, which later generation
slices are expected to follow:

> An annotation that contradicts the ent schema fails generation, reporting
> both facts. Anything that can be generated correctly is generated, not
> refused.

Two problems are detected today. The first is an ent-`Immutable()` field carrying
`ScopeUpdate` (which `DefaultField()` grants). ent's update builders iterate
`MutableFields`, which excludes immutable fields, so `Set<X>` does not exist on
`<Entity>UpdateOne` and no template can emit a call that compiles. Dropping the
field silently was rejected: it would vanish from the PATCH API where neither
`encoding/json` nor `Validate()` can observe the missing key.

There used to be a second: an entity whose primary key did not render as
`uuid.UUID`, refused whenever `WithBaseService` or `WithBaseHandler` was on. Both
templates and both options are gone (#29), so `checkGraphConflicts` no longer
takes the config and no identifier type is refused. Every check that remains is
unconditional, and all of them are skipped for a node with no domain fields —
matching the condition the generation loop itself uses.

Conversely, `Optional().Nillable()` and named `GoType`s over slices/maps *are*
generated, because correct output exists for them — `*T` in the create request
(`dto.tmpl`), and `PtrNilSafe` chosen by `isComplexFieldType`, which reads
`field.Type.RType.Kind` rather than the rendered type name. The full table is
in README.md, "Field shapes".

Every row has a fixture. A fixture whose generation must fail carries
`wantGenErr` in the `fixtures` table and has no generated output to commit; see
`internal/fixtures/README.md`.

## Testing conventions

- Tests are in-package (`package entdomain`) in both packages. Generator tests build `gen.Field`/`gen.Type` values by hand via the constructors in `test_helpers_test.go` (`newStringField`, `newUUIDField`, `newTestType`, `ptr`, `assertContains`, …). Use those instead of hand-rolling literals. A test for a runtime symbol belongs in `runtime/`, and must not pull ent in — that is what keeps the runtime testable without the generator's dependency graph.
- `funcs_codegen.go` helpers are tested by asserting on **substrings of emitted Go source**, not by compiling it.
- Two tests do render templates end-to-end: `TestCodegenFixtures` (generates into `internal/fixtures/<dir>/<dir>ent` and compiles the result) and `TestTemplatesDeclareTheirImports` (renders each template and checks goimports changes no import). A template edit that breaks compilation or import declarations is caught here; anything beyond that still needs a real ent project.
- **The fixture harness contract: one directory plus one line.** Add `internal/fixtures/<dir>/<dir>ent/schema/` with a hand-written ent schema, and one `{dir: "<dir>"}` line in the `fixtures` table in `codegen_fixtures_test.go`. Nothing else in the harness changes. A schema the generator must **refuse** is the same directory plus `wantGenErr: []string{…}`: generation is then required to fail, its error must contain every listed substring, and nothing may be written under `<dir>/<dir>ent` besides the hand-written schema. "It failed" is not the assertion — the message is, because the message is all a schema author gets.
- **The generated directory is `<dir>ent`, never `ent` (#49).** A Go package is named after the last element of its import path, so `internal/fixtures/basic/basicent` declares `package basicent`. That is deliberate and load-bearing: goimports resolves a bare `ent.` reference by **package name**, and when thirteen fixtures all declared `package ent` the winner depended on its module index cache — which is how one run rewrote `entgo.io/ent` in `softdelete`'s `client.go` to `basic`'s package. The harness derives every path from `entDirName`/`fixtureEntPkgPath` in `codegen_fixtures_test.go`, and `TestNoAmbiguousEntPackages` fails the run if any package under `internal/fixtures` is named `ent`.
- `TestCodegenFixtures` **writes into the repository tree** on purpose: generated ent code has to sit inside this module to resolve `github.com/githonllc/entdomain` without a replace directive, and `t.TempDir()` is outside any module. The output is committed, so a clean checkout plus a test run leaves `git status` clean. **A dirty tree after `make check` means generation changed** — regenerate, never hand-edit anything under `internal/fixtures/<dir>/<dir>ent/`.
- **`internal/fixtures` (plural) is the generator's output; `internal/fixture` (singular) is its target.** The singular one is a separate Go module holding the #22 spike: `ent/dto/` there is hand-written, compiled and exercised against SQLite, and it is the specification. Read it; never edit it — except for a mechanical migration every consumer also has to make, which is how #15 changed exactly one import line in six of its files.
- **Three nested modules run under `make test-modules`, and each exists because a compile proof cannot answer its question.** They are separate modules because they need a SQL driver and this library must not have one.
  - `internal/fixture` — the #22 spike. It breaking is the signal that generated output has drifted from the hand-written target.
  - `internal/fixtures/wiring/e2e` — the behavioural half of the wiring fixture. Compiling proves the wiring type-checks; only this proves it returns the right page, and it carries #13's three-way missing-row / `UNIQUE` / `FOREIGN KEY` mapping proof against real SQLite.
  - `internal/softdeleteproof` — the only evidence for #18's load-bearing claim: a direct `client.Doc.Query()`, with nothing generated in the call path, does not return deleted rows. A compile proof cannot tell "the predicate is generated" from "the predicate reaches the SQL".
- `internal/fixtures/edges/edgesent/orerr_contract_test.go` is one of the few hand-written files under a generated `<dir>ent/` directory. It must be in `package edgesent` because it sets the unexported `Edges.loadedTypes`, which is the only way to construct *loaded and absent* without a database. The others are `basic/basicent/listresponse_shape_test.go`, `presence/presenceent/account_presence_test.go`, `query/queryent/filter_contract_test.go` and `stale/staleent/trinket_dto.go`; `go build` ignores `_test.go` files, so only the last of those is visible to the codegen harness — and it is there precisely to prove cleanup leaves files it did not write alone.
- The two nested modules that import a fixture package (`internal/fixtures/wiring/e2e`, `internal/softdeleteproof`) alias it back to `ent` in their imports. They are hand-written and read from the consumer's side, where the generated package really is named `ent`; the alias is spelled out, so goimports has nothing to resolve.

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
absence check. `runtimePackage` is bound to `defaultEntDomainPackage`, so it
cannot drift from what generated code actually imports. Write new absence
assertions the same way: an absence check without a positive control is a rubber
stamp waiting to happen. `runtime/runtime_deps_test.go` is the cheap parser-level
half, reading the package directory rather than a hand-maintained file list.

A fifth, one level up: `TestRemovedTemplatesStayRemoved` (`template_loader_test.go`)
pins that `base_service.tmpl` and `base_handler.tmpl` are gone **and** that
`generatedFileNames` still lists the two file names they produced. Cleanup is
the only thing that deletes them from a consumer's tree, so the name has to
outlive the template.

Related: `templates/dto.tmpl` calls `IsNotFound` **unqualified on purpose** —
the emitted file lands in the consumer's `package ent`, so it binds to Ent's
generated predicate, not to the runtime's `IsNotFound` (`runtime/errors.go`).
Qualifying it would compile and silently stop matching, routing every
loaded-but-absent edge into the error branch.
`TestDTOTemplateResolvesIsNotFoundToEnt` pins that direction, and
`TestTemplatesQualifyEntdomainSentinels` pins the converse across every
template: `entdomain.Err*` always stays qualified, because `package ent` has no
such symbol.

### Baseline state

`make check` (including `test-modules`), `go test ./...`, `gofmt -l .` and `make lint` are all green on a clean checkout, and `git status --porcelain` is empty after `make check`. **Anything you see is yours.**

`golangci-lint` and `goimports` are not on the default PATH; run lint as `PATH=$PATH:$HOME/go/bin make lint`. It must be **golangci-lint v1** — v2 rejects this repo's `.golangci.yml` schema.

`gofmt -l .` covers the whole tree, but `make fmt` deliberately does not: `FMT_FILES` excludes `internal/fixture/` and `internal/fixtures/`, because gofmt walks the filesystem rather than the module graph and would otherwise rewrite generated files that the next test run rewrites back. Hand-written files under those prefixes — the fixture schemas, and the hand-written `_test.go` files inside generated `ent/` directories — must be kept gofmt-clean by hand.

## Docs to keep in sync

`README.md` and `README_zh.md` are parallel translations; changing the public API means editing both, and both carry the migration notes for every symbol this module has removed. There are now **two** `doc.go`s: the root one is the generator's godoc quick start and the runtime migration pointer, `runtime/doc.go` is the runtime's. `ARCHITECTURE.md` carries the module table and the PlantUML diagrams, and `.claude/skills/entdomain/SKILL.md` documents downstream usage patterns (some of it describes a consumer project's interceptors, not this repo).

Moving or removing a published symbol is an established move here, not an exception — #3, #6, #24, #26, #29 and #15 all did it. The convention is a migration note in **both** READMEs plus a `doc.go` pointer, and no compatibility alias: an alias that preserves the coupling a change exists to remove is worse than the break.
