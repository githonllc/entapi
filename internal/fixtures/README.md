# fixtures — the codegen harness

Read this first if you arrived from `internal/fixture` (singular). **They are two
different things and the names are one character apart.**

| | `internal/fixture` (singular) | `internal/fixtures` (plural, here) |
|---|---|---|
| Module | separate Go module, own `go.mod`, SQLite driver | part of `github.com/githonllc/entdomain` |
| ent code produced by | `go generate ./ent`, **without** this extension | `TestCodegenFixtures`, **with** this extension |
| Domain layer | hand-written (`ent/dto/user.go`) as the generator's *target* | generated, as the generator's *output* |
| Question it answers | "what should the generator emit?" | "does what the generator emits compile?" |

## How this one works

`../../codegen_fixtures_test.go` walks the `fixtures` table, runs `entc.Generate`
over `<dir>/ent/schema` with the entdomain extension installed, and then shells
out to `go build ./internal/fixtures/<dir>/...`. A non-zero build fails the test
with the compiler's own output.

The generated code has to live inside this module so it can resolve
`github.com/githonllc/entdomain` with no network fetch and no `replace`
directive. So **the test writes into the repository tree**, and the output is
committed. A clean checkout plus a test run leaves `git status` clean; a dirty
tree means generation changed, not that the test misbehaved.

Everything under `<dir>/ent/` except `<dir>/ent/schema/` is generated and carries
a DO NOT EDIT header. Regenerate; never hand-edit. Three files are deliberate
exceptions, and each says so in its own header:

- `stale/ent/trinket_dto.go`, hand-written to prove cleanup keys on entdomain's
  marker rather than on a file name.
- `presence/ent/account_presence_test.go`, hand-written and in `package ent`. It
  asserts what compilation cannot: that an omitted field is never written to the
  builder's mutation, so ent's `Default()` still applies, and that a patch tells
  absent from explicit null from value. ent records every `Set`/`Clear` on the
  mutation, which is the last observable point before the query — and the only
  one available in a module with no SQL driver.
- `edges/ent/orerr_contract_test.go`, hand-written and in `package ent`. It has
  to be, because the contract it pins is exactly that `Edges.loadedTypes` is
  unreachable from anywhere else — setting that flag is the only way to build the
  *loaded and absent* state without a database, and this module has no SQL driver
  on purpose. `go build` ignores test files, so the harness is unaffected;
  `go test ./...` from the repository root compiles and runs it.

## Adding a fixture

1. `mkdir -p internal/fixtures/<name>/ent/schema` and write one ent schema there.
2. Add one line to the `fixtures` table in `codegen_fixtures_test.go`.
3. `go test -run TestCodegen ./.` and commit the generated output.

`basic` is the happy path and must stay green. Hostile shapes belong to the issue
that fixes the bug they expose, each bringing its own directory — do not make
`basic` hostile.

## Fixtures whose generation must FAIL

Some schemas are legal ent but have no correct output at all — an `Immutable()`
field carrying `ScopeUpdate` is the standing example, since ent generates no
update setter for one. The generator refuses those rather than emitting
something that does not build, and the refusal needs coverage too.

Set `wantGenErr` on the table entry to a list of substrings the generation error
must contain. The case then asserts three things: generation failed, the message
contains each substring (naming the entity, the field and the conflict — "it
failed" is not enough, the message is all a schema author gets), and **nothing
was written** under `<dir>/ent` except the hand-written `schema` package. So
such a fixture has no generated output to commit.

`go test -v -run TestCodegenFixtures/<dir> ./.` prints the refusal message even
when the case passes.

| Fixture | Shape | Outcome |
|---|---|---|
| `basic` | happy path: create/update/response scopes, uuid id, one time field | generates and compiles |
| `fieldshapes` | nillable, enum, JSON/map and named-`GoType` fields, optional and required | generates and compiles |
| `edges` | to-one, to-many, a self-referential pair declared separately, a response-scoped foreign key whose edge is deliberately unannotated, and an entity with no response-scoped field at all | generates and compiles |
| `selfrefpartial` | a self-referential pair exposing one end only, the other end holding a bare `entdomain.Edge()` | generates and compiles |
| `presence` | a defaulted field, an omitted optional field, an explicit null, a required non-string field, an `Immutable()` create-only field | generates, compiles, and is exercised by `ent/account_presence_test.go` |
| `immutable` | `Immutable()` fields carrying `ScopeUpdate` | generation refused |
| `intid` | a domain-annotated entity with ent's default `int` primary key, with the base service enabled | generation refused |
| `selfref` | a self-referential pair declared in the chained form, so annotated on one end only | generation refused |
| `stale` | an entity that loses its annotations between two runs | generates twice, see below |

## The self-referential pair, in three fixtures

`edges`, `selfref` and `selfrefpartial` are one story told three times, and they
only make sense together.

Written in the chained form, `edge.To("children", T.Type).From("parent")...Annotations(a)`
hands the annotation to the **inverse** edge alone. The assoc edge `children` is
left bare — and a bare edge is exactly how a schema author says *do not expose
this*. Before #30 the two were indistinguishable: generation succeeded, the
output compiled, and `children` was in no response type and no eager-load plan,
with no message anywhere.

- `edges/Category` declares the pair as two separate edges, both annotated: both
  ends reach `CategoryResponse` and the plan is `WithChildren().WithParent()`.
- `selfref/Tree` is the chained form. Generation is now refused, naming both
  ends and the fix.
- `selfrefpartial/Node` exposes `parent` alone on purpose and writes that down
  with a bare `entdomain.Edge()` on `children`. It generates, and `NodeResponse`
  carries `Parent` and no `Children` — the same output the trap produced, now
  reached deliberately.

`selfrefpartial` has to be a real generation rather than a unit test: the claim
at risk is that an *empty* annotation survives the schema load, which reaches
codegen through a JSON round-trip, and `DomainEdge{}` marshals to `{}`. If ent
dropped it, the refusal message would be recommending a fix that does not work.

`basic`, `fieldshapes`, `edges` and `presence` are also the corpus for
`TestTemplatesDeclareTheirImports`, which renders each template over them and
fails if goimports has to add or remove an import — that is what keeps the
formatter a safety net rather than the mechanism. Every refused fixture is
deliberately absent from it: generation stops before any template is rendered,
so there is nothing to check.

## The `stale` fixture

`stale` is the one fixture that does not fit the table, because it needs two
generation runs over one target directory: `stale/annotated/schema` has
annotations on `Sprocket`, `stale/plain/schema` is the same schema without them,
and `TestCodegenFixtureStaleArtifacts` runs them in that order. What is committed
under `stale/ent/` is the second run's result, so a passing run still leaves
`git status` clean.

`stale/ent/trinket_dto.go` is hand-written and carries ent's generic
`Code generated by ent, DO NOT EDIT.` header. It occupies a file name cleanup
considers for deletion, so it is what proves cleanup keys on entdomain's own
marker rather than on the file name or on "Code generated".
