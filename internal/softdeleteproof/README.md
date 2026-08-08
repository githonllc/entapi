# softdeleteproof — the behavioural half of #18

There are now three directories one character apart. This is the third.

| | `internal/fixture` | `internal/fixtures` | `internal/softdeleteproof` |
|---|---|---|---|
| Module | separate, SQLite | this one | separate, SQLite |
| ent code | `go generate`, no extension | `TestCodegenFixtures`, with the extension | **none of its own** |
| Question | what should the generator emit? | does the output compile? | **does the output do what it claims?** |

It owns no schema and no generated code. It imports
`internal/fixtures/softdelete/softdeleteent` — generated and committed by
`TestCodegenFixtures` — and adds the one thing the parent module deliberately
does not have: a SQL driver.

That split is the whole point. The claim #18 rests on is that soft delete is
only enforceable at ent's interceptor layer, because one line of ordinary
consumer code — `client.Doc.Query().All(ctx)` — reaches the database with
nothing entapi-generated in that query call path. #70 injects the interceptor
and hook into Ent's own `newConfig`, so the consumer writes no registration
call. This module proves that placement through plain `NewClient`, `Open`,
`enttest.Open`, two simultaneous clients, and a deny-by-default privacy policy.
A compile-only proof cannot distinguish "the predicate is generated" from
"the predicate reaches the SQL", so the assertions here read rows back against
real ent and real SQLite.

`modernc.org/sqlite` stays out of `github.com/githonllc/entdomain`'s dependency
graph: this module's `go.mod` is its own, and the root one is untouched.

## Running

```
cd internal/softdeleteproof && go test ./...
```

It is not reached by `go test ./...` at the repository root — Go excludes nested
module directories from a parent module's package patterns — so it has to be run
on its own. Run it after `go test -run TestCodegen ./.`, which is what produces
the code it compiles against.
