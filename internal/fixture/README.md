# fixture

A real ent project used to prove that the shapes entdomain intends to generate
actually compile and run.

It is a **separate Go module** on purpose: its SQLite driver must never enter
the library's dependency graph. Consumers of `github.com/githonllc/entdomain`
are unaffected by anything in this directory.

## What is hand-written, and why

`ent/dto/user.go` is written by hand. It is the *target output* of the
generator, not output of it.

Writing the target first — then compiling it, running it, and reading it — is
the opposite of how the current templates were built, and it is the reason
those templates emit code no test ever compiled. Only once this file is known
to be correct is it worth teaching a template to produce it.

Everything in that file is mechanical: each declaration follows from the schema
and from ent's own generated API. Nothing in it required a judgement call,
which is the test for whether it belongs in generated code at all.

## Layout

| Path | Layer | Written by |
|---|---|---|
| `ent/schema/user.go` | schema | hand |
| `ent/*` (except `dto/`) | ent's own output | `go generate ./ent` |
| `ent/dto/user.go` | **Layer 1** — per-entity, schema-derived | hand, as the generator's target |
| `../../query.go` | **Layer 2** — CRUD algorithms | hand, once, for all entities |
| `e2e_test.go` | proof | hand |

## Running

```
go generate ./ent    # regenerate ent's own code
go test ./...        # compile Layer 1 + Layer 2 and exercise them against SQLite
```

## What the tests establish

- A generic `ListPage` written once drives a real `*ent.UserQuery`, with full
  type inference at the call site and no per-entity algorithm.
- The identifier type is a type parameter, so nothing pins it to `uuid.UUID`.
- `Filterable` / `Searchable` / `Sortable` compose into one query: per-field
  predicates AND together, free-text search ORs across searchable fields and
  then ANDs with the rest, and ordering is checked against an allow-list.
- A field omitted from a create request is not written, so the schema's
  `Default()` applies.
- PATCH distinguishes absent, explicit null and value; only optional fields can
  be cleared, and an explicit null on a non-optional field is rejected.
- An `InputOnly` field cannot reach a response.
