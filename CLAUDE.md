# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make check                  # fmt + vet + test (run before committing)
make test                   # go test -count=1 -v ./...
make cover                  # coverage summary (CONTRIBUTING targets >85%)
make lint                   # golangci-lint run ./...
make fmt                    # gofmt + goimports -local github.com/githonllc/entdomain

go test -run TestCamelCase -v ./.          # single test
go test -run 'TestFieldPredicate_.*' ./.   # subset by regex
```

Note: the Makefile overrides `GOPATH=/tmp/gopath` and `GOMODCACHE=/tmp/gomodcache`. Bare `go test ./...` uses your normal module cache and is equivalent otherwise.

## What this repository is

A single Go package (`entdomain`) that plays **two distinct roles**, both from the same package:

1. **Code-generation time** — an [Ent](https://entgo.io) extension. Consumers wire it into their `entc.go`; it reads `DomainField` annotations off `gen.Field`s and writes `{entity}_dto.go`, `{entity}_base_service.go`, `{entity}_base_handler.go` into the consumer's `ent/` package.
2. **Runtime** — the types the generated code links against: `PageInfo`/`Cursor`, `ListRequest`, `ErrNotFound`/`ErrAlreadyExists`/`ErrValidation`, and `Ptr`/`PtrOrNil`/`PtrNilSafe`.

There is no `main`, no example app, and no downstream ent project in this repo. Anything about *generated* code can only be verified by generating into a real ent project.

## Generation pipeline

`extension.go` is the whole entry point:

- `Hooks()` returns one `gen.Hook` (`generatePerTypeFiles`). **`Templates()` deliberately returns an empty slice** — this extension does not use Ent's `GraphTemplate` mechanism. Do not "fix" that by adding templates there.
- The hook runs *after* `next.Generate(g)`, then loops `g.Nodes`. **A node with zero `domainFields` is skipped entirely** — that's how unannotated entities avoid producing empty files.
- Each file is rendered with `text/template`, then run through `golang.org/x/tools/imports` in `writeFile`. Formatting failure is a logged warning, not an error: the unformatted source is written anyway.
- `templateFuncMap()` layers three sources, later wins: Ent's `gen.Funcs` → this package's `templateFuncs()` → `entdomainPkg` (closure over the configured import path).

### Templates

`templates/*.tmpl` are embedded via `//go:embed` (`template_loader.go`) and loaded into package-level vars by `template_index.go` using `mustLoadTemplate`, i.e. **at package init — renaming or deleting a template panics on import**, not at generation time.

Templates receive a `*gen.Type` as `.`, so `$.Config.Package`, `$.Package`, `$.Name`, `$.ID` and the standard Ent template funcs are all available.

### Template functions

Split by concern across `funcs_*.go`, and registered in one map in `funcs.go`:

| File | Contains |
|---|---|
| `funcs_fields.go` | field selection: `domainFields`, `createFields`, `updateFields`, `responseFields`, `responseEdges`, lookup-field filters |
| `funcs_scope.go` | `hasDomainScope`, `isDomainRequired`, and `getDomainFieldAnnotation` |
| `funcs_typechecks.go` | `isTimeField`, `hasTimeFields`, `hasSoftDelete`, `isComplexFieldType` |
| `funcs_codegen.go` | string-emitting helpers (`setFieldCallReq`) |
| `funcs_strings.go` | `camelCase`, `contains`, `hasPrefix` |

**A helper is only callable from a template if it appears in `templateFuncs()`.** Adding a func to a `funcs_*.go` file is not enough.

### Annotation access

Never read `field.Annotations["DomainField"]` directly. Always go through `getDomainFieldAnnotation` (`funcs_scope.go`): the annotation arrives as `*DomainField` during codegen but as `map[string]interface{}` when loaded from a serialized schema, and that function normalizes both via a JSON round-trip.

## Annotation model

`annotations.go` defines `DomainField` plus value-receiver fluent builders (`WithRequired`, `AsSearchable`, `AsUniqueLookup`, `WithFormat`, …). Every builder **returns a copy** — chaining works, mutating in place does not.

Preset builders (`DefaultField`, `InputOnlyField`, `OutputOnlyField`, `CreateOnlyField`, `IdField`, `AuditLogField`) are just scope combinations layered on `DomainFieldWithScopes`.

The load-bearing design rule, repeated throughout the code and README: **scopes only control HTTP-layer struct generation.** They never restrict what the service layer can do with an ent entity. Keep new features on that side of the line.

## Conventions baked into generated code

- `base_service.tmpl` hardcodes `uuid.UUID` as the entity ID type in hook signatures and CRUD methods. Non-UUID primary keys are not supported by the current BaseService.
- Soft delete is **convention-based**: `hasSoftDelete` matches a `Nillable` `time.Time` field literally named `deleted_at`, and switches Delete to `UpdateOneID().SetDeletedAt(now)`.
- Hook dispatch works via `SetSelf` + an embedded interface; without `SetSelf` the no-op defaults on `Base{Entity}Service` are used.
- `Base{Entity}Handler` exists so consumer handler packages never import `ent` transitively for conversion — keep it dependency-free.

## Generation can fail, and that is a feature (#10)

`generatePerTypeFiles` calls `checkGraphConflicts` (`schema_conflicts.go`)
**before** `next.Generate(g)`, so a rejected schema leaves nothing on disk —
not even ent's own output. The policy it implements, which later generation
slices are expected to follow:

> An annotation that contradicts the ent schema fails generation, reporting
> both facts. Anything that can be generated correctly is generated, not
> refused.

The one contradiction detected today is an ent-`Immutable()` field carrying
`ScopeUpdate` (which `DefaultField()` grants). ent's update builders iterate
`MutableFields`, which excludes immutable fields, so `Set<X>` does not exist on
`<Entity>UpdateOne` and no template can emit a call that compiles. Dropping the
field silently was rejected: it would vanish from the PATCH API where neither
`encoding/json` nor `Validate()` can observe the missing key.

Conversely, `Optional().Nillable()` and named `GoType`s over slices/maps *are*
generated, because correct output exists for them — `*T` in the create request
(`dto.tmpl`), and `PtrNilSafe` chosen by `isComplexFieldType`, which reads
`field.Type.RType.Kind` rather than the rendered type name. The full table is
in README.md, "Field shapes".

Every row has a fixture. A fixture whose generation must fail carries
`wantGenErr` in the `fixtures` table and has no generated output to commit; see
`internal/fixtures/README.md`.

## Testing conventions

- Tests are in-package (`package entdomain`) and build `gen.Field`/`gen.Type` values by hand via the constructors in `test_helpers_test.go` (`newStringField`, `newUUIDField`, `newTestType`, `ptr`, `assertContains`, …). Use those instead of hand-rolling literals.
- `funcs_codegen.go` helpers are tested by asserting on **substrings of emitted Go source**, not by compiling it.
- **No test renders a full template end-to-end.** Template edits are effectively untested here; verify them by regenerating in a real ent project.

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

Related: `templates/base_service.tmpl` calls `IsNotFound`/`IsConstraintError`
**unqualified on purpose** — the emitted file lands in the consumer's
`package ent`, so those bind to Ent's generated predicates, not to this
package's `IsNotFound` (`errors.go`). Qualifying them would compile and
silently stop matching. `TestGeneratedErrorPredicatesResolveUnambiguously`
guards both that and the converse rule that the `entdomain.Err*` sentinels
always stay qualified.

### Baseline state

`make check` and `gofmt -l .` are green; the red suite and dirty formatting the initial commit shipped with were fixed in #2/#4/#5. `make lint` still exits 2 on four `unused` findings in `annotations_edge.go` — those functions are awaiting the tests in #24. Anything else is yours.

`golangci-lint` and `goimports` are not on the default PATH; run lint as `PATH=$PATH:$HOME/go/bin make lint`.

## Docs to keep in sync

`README.md` and `README_zh.md` are parallel translations; changing the public API means editing both. `doc.go` carries the package-level godoc quick start, and `.claude/skills/entdomain/SKILL.md` documents downstream usage patterns (some of it describes a consumer project's interceptors, not this repo).
