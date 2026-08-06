# fixture

A real ent project used to prove that the shapes entdomain intends to generate
actually compile and run.

It is a **separate Go module** on purpose: its SQLite driver must never enter
the library's dependency graph. Consumers of `github.com/githonllc/entdomain`
are unaffected by anything in this directory.

## What is hand-written, and why

`spikeent/dto/user.go` is written by hand. It is the *target output* of the
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
| `spikeent/schema/user.go` | schema | hand |
| `spikeent/*` (except `dto/`) | ent's own output | `go generate ./spikeent` |
| `spikeent/dto/user.go` | **Layer 1** — per-entity, schema-derived | hand, as the generator's target |
| `../../query.go` | **Layer 2** — CRUD algorithms | hand, once, for all entities |
| `e2e_test.go` | proof | hand |

The generated directory is `spikeent`, not `ent`, and that is not cosmetic
(#49). goimports resolves a bare `ent.` reference by PACKAGE NAME against every
candidate it finds by walking the filesystem — it never consults the module
graph, so this module being a separate one hides nothing from it.
`entgo.io/ent` is already `package ent`, so a second one here made the choice
depend on goimports' cache state, and it rewrote `entgo.io/ent` in a *different*
module's generated `client.go` to point at this directory. `package spikeent`
cannot be picked by mistake. `TestNoAmbiguousEntPackages` in the root module
censuses the whole repository tree, nested modules included, and fails if any
package named `ent` reappears.

## Running

```
go generate ./spikeent   # regenerate ent's own code
go test ./...            # compile Layer 1 + Layer 2 and exercise them against SQLite
```

## What the tests establish

- A generic `ListPage` written once drives a real `*spikeent.UserQuery`, with full
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

## Edges

`Post` and a self-referential `Category` exist to exercise the cases `User`
alone cannot.

- A **to-many** edge is reachable. Under the old rule it could not be: that rule
  requires `edge.Field() != nil`, which holds only when the foreign key sits on
  this entity, so `User.posts` was permanently unreachable. Edges now carry their
  own annotation.
- `author_id` and `author` are **independent**. The scalar comes from the field's
  scope, the nested object from the edge's annotation. The old rule made them one
  switch.
- Edge state is read through `<Edge>OrErr()`, never a nil check. `loadedTypes` is
  unexported, so a nil pointer cannot distinguish *not loaded* from *loaded and
  absent*. Loaded-and-absent serializes as an explicit `null`; not-loaded is an
  error.
- **Eager-load plans are generated from the response type's edge set**, so a
  caller cannot forget one. That is what makes "not loaded is an error" cheap:
  in generated wiring it never fires, and it only ever catches a hand-rolled
  query. Note that `db.User.Get` cannot serve a response with edges — it loads
  none.
- Two-tier types bound expansion at depth 1 **in the type system**: a summary has
  no edge fields, so a cycle cannot be built and no depth counter is needed. The
  cost is explicit in `TestTwoTierBoundsDepthAndWhatThatCosts` — a three-level
  tree comes back one level deep.

### A trap worth knowing

Declaring a self-referential pair in the chained form

```go
edge.To("children", X.Type).From("parent").Unique().Field("parent_id").Annotations(a)
```

attaches the annotation to the **inverse** edge only. The assoc edge is left
unannotated, nothing is reported, and it silently never appears in a response.
`category.go` declares the two edges separately for this reason, and
`schema_annotation_test.go` guards it.

### Inspecting the graph

`spikeent/probe.go` (build-tagged `ignore`) prints every edge with its annotation,
its uniqueness and whether `edge.Field()` is nil:

```
go run -mod=mod probe.go
```
