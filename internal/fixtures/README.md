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
a DO NOT EDIT header. Regenerate; never hand-edit.

## Adding a fixture

1. `mkdir -p internal/fixtures/<name>/ent/schema` and write one ent schema there.
2. Add one line to the `fixtures` table in `codegen_fixtures_test.go`.
3. `go test -run TestCodegen ./.` and commit the generated output.

`basic` is the happy path and must stay green. Hostile shapes belong to the issue
that fixes the bug they expose, each bringing its own directory — do not make
`basic` hostile.
