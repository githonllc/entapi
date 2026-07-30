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
| `funcs_typechecks.go` | `isTimeField`, `isUUIDType`, `hasSoftDelete`, `isComplexFieldType` |
| `funcs_codegen.go` | string-emitting helpers (`setFieldCallReq`, `fieldPredicate`, …) |
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

## Testing conventions

- Tests are in-package (`package entdomain`) and build `gen.Field`/`gen.Type` values by hand via the constructors in `test_helpers_test.go` (`newStringField`, `newUUIDField`, `newTestType`, `ptr`, `assertContains`, …). Use those instead of hand-rolling literals.
- `funcs_codegen.go` helpers are tested by asserting on **substrings of emitted Go source**, not by compiling it.
- **No test renders a full template end-to-end.** Template edits are effectively untested here; verify them by regenerating in a real ent project.

## Known dead / stale code (verified as of the initial commit)

Do not treat these as load-bearing, and prefer deleting over extending them:

- `templates/model.tmpl` is byte-identical to `templates/dto.tmpl` except for one header comment, and is never loaded by `template_index.go`. `dto.tmpl` is the live one. (`.claude/skills/entdomain/SKILL.md` still points at `model.tmpl` — that line is wrong.)
- `generateIdOperation`, `generateSearchCondition`, `searchMethod`, `findByMethod`, `isUniqueField`, `isUUIDType`, `uniqueLookupFields`, `rangeLookupFields` are registered as template funcs but referenced by **no** template. The first four emit repository-era code (`r.client…`, `model.…`) that no longer matches the generated `BaseService` shape (`s.DB`). The comment in `funcs.go` claiming only template-invoked functions are registered is therefore inaccurate.
- `funcs_test.go` defines its own `convertMapToDomainField`, a test-local duplicate of the map branch of `getDomainFieldAnnotation`.

### Baseline state at the initial commit

`go test ./...` is **red on a clean checkout**: `TestTemplateFuncs` asserts the presence of `specificMethods` and `setFieldCall`, which no longer exist. `gofmt -l .` also flags `funcs.go`, `funcs_codegen.go`, `annotations_test.go`, `types_test.go`, so `make lint` fails too. If you see these, they predate your change — but do not add to them.

## Docs to keep in sync

`README.md` and `README_zh.md` are parallel translations; changing the public API means editing both. `doc.go` carries the package-level godoc quick start, and `.claude/skills/entdomain/SKILL.md` documents downstream usage patterns (some of it describes a consumer project's interceptors, not this repo).
