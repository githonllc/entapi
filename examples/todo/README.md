# EntAPI todo example

A complete, runnable service: one ent schema, one `go generate`, one `go run`.
Everything between them — DTOs, filters, wiring, HTTP handlers, the error map
and `openapi.yaml` — is generated.

## Three commands

```bash
cd examples/todo
go generate ./...
go run .
```

The service listens on `http://localhost:8080` and prints its own endpoint
manifest at startup — that list comes from `api.Endpoints()`, not from a
hand-written table:

```
GET    /todos
POST   /todos
GET    /todos/{id}
PATCH  /todos/{id}
DELETE /todos/{id}
GET    /openapi.yaml
listening on http://localhost:8080
```

The database is in memory, so every run starts empty and nothing is written to
disk. To keep the data, swap the DSN in `main.go` for `file:todo.db?_fk=1`.

## What you wrote, and what you got

You wrote four files:

| File | What it is |
|---|---|
| `todoent/schema/todo.go` | the ent schema, plus five `api.…()` annotation words |
| `todoent/entc.go` | the `entc.Generate` call that installs the extension |
| `todoent/generate.go` | the one-line `//go:generate` directive that runs `entc.go`. Easy to forget, and without it `go generate ./...` silently does nothing |
| `main.go` | open a DB, migrate, `todoent.API(client)`, serve |

The whole API surface comes from four annotated fields:

```go
field.String("title").Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
field.Bool("done").Optional().Default(false).Annotations(api.Filterable()),
field.Int("priority").Optional().Default(0).Annotations(api.Filterable(), api.Sortable()),
field.Time("created_at").Immutable().Default(time.Now).Annotations(api.Sortable(), api.ReadOnly()),
```

`api.Resource()` on the entity is the switch that turns generation on at all.
`api.ReadOnly()` on `created_at` is why it appears in every response but in
neither request.

## Exercising it

Every response body below is pasted verbatim from a real run of these exact
commands against a freshly started server. The status code in front of each body
is not part of the body — `curl -s` does not print it; add
`-w '%{http_code}\n'` if you want to see it too.

### Create

```bash
curl -s -X POST localhost:8080/todos -H 'Content-Type: application/json' \
  -d '{"title":"buy milk","priority":2}'
curl -s -X POST localhost:8080/todos -H 'Content-Type: application/json' \
  -d '{"title":"write docs","priority":5}'
curl -s -X POST localhost:8080/todos -H 'Content-Type: application/json' \
  -d '{"title":"ship the example","priority":1,"done":true}'
```

```
201 {"id":1,"title":"buy milk","priority":2,"created_at":"2026-08-13T06:05:49.366589-07:00"}
201 {"id":2,"title":"write docs","priority":5,"created_at":"2026-08-13T06:05:49.376439-07:00"}
201 {"id":3,"title":"ship the example","done":true,"priority":1,"created_at":"2026-08-13T06:05:49.384963-07:00"}
```

### List

```bash
curl -s localhost:8080/todos
```

```
200 {"data":[{"id":1,"title":"buy milk","priority":2,"created_at":"2026-08-13T06:05:49.366589-07:00"},{"id":2,"title":"write docs","priority":5,"created_at":"2026-08-13T06:05:49.376439-07:00"},{"id":3,"title":"ship the example","done":true,"priority":1,"created_at":"2026-08-13T06:05:49.384963-07:00"}],"total":3,"page":1,"size":20}
```

### Filter and sort

`done=false` is a filter parameter derived from `api.Filterable()`;
`_sort=priority:desc` is checked against the `api.Sortable()` allow-list.

```bash
curl -s 'localhost:8080/todos?done=false&_sort=priority:desc'
```

```
200 {"data":[{"id":2,"title":"write docs","priority":5,"created_at":"2026-08-13T06:05:49.376439-07:00"},{"id":1,"title":"buy milk","priority":2,"created_at":"2026-08-13T06:05:49.366589-07:00"}],"total":2,"page":1,"size":20}
```

### Free-text search

`_q` searches every `api.Searchable()` field — here, just `title`.

```bash
curl -s 'localhost:8080/todos?_q=milk'
```

```
200 {"data":[{"id":1,"title":"buy milk","priority":2,"created_at":"2026-08-13T06:05:49.366589-07:00"}],"total":1,"page":1,"size":20}
```

### Comparison operators and paging

A bare value means `eq`; anything else is written `<operator>:<value>`. The set
each field accepts is in `openapi.yaml`. `total` is the unpaged count.

```bash
curl -s 'localhost:8080/todos?priority=ge:2&_size=1'
```

```
200 {"data":[{"id":1,"title":"buy milk","priority":2,"created_at":"2026-08-13T06:05:49.366589-07:00"}],"total":2,"page":1,"size":1}
```

### Get one

```bash
curl -s localhost:8080/todos/2
```

```
200 {"id":2,"title":"write docs","priority":5,"created_at":"2026-08-13T06:05:49.376439-07:00"}
```

### Patch

A key you omit is left alone; `created_at` is not accepted at all.

```bash
curl -s -X PATCH localhost:8080/todos/2 -H 'Content-Type: application/json' \
  -d '{"done":true,"priority":9}'
```

```
200 {"id":2,"title":"write docs","done":true,"priority":9,"created_at":"2026-08-13T06:05:49.376439-07:00"}
```

### Delete, then ask for it again

```bash
curl -s -X DELETE localhost:8080/todos/1
curl -s localhost:8080/todos/1
```

```
204 (no body)
404 {"type":"about:blank","title":"Not Found","status":404,"detail":"entity not found: todoent: todo not found"}
```

### Errors are RFC 9457 problem documents

`title` is required by the schema and has no default, so omitting it is a 422,
not a 500 — served as `application/problem+json`.

```bash
curl -s -X POST localhost:8080/todos -H 'Content-Type: application/json' \
  -d '{"priority":3}'
```

```
422 {"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"validation failed: title is required"}
```

### The OpenAPI document

Served from an `//go:embed` of the generated `openapi.yaml`, as
`application/yaml`. 264 lines, all derived from the same schema.

```bash
curl -s localhost:8080/openapi.yaml | head -22
```

```
# Code generated by entapi extension for this schema as a whole. DO NOT EDIT.
# Source template: templates/openapi.tmpl
# Regenerate with: make generate
#
# Delete the line above and this file is yours, permanently: cleanup identifies
# what it may remove by that marker and by nothing else.
#
# There is deliberately no `servers` entry. A mount prefix is a deployment fact
# — http.StripPrefix runs in the consumer's main, long after generation, and one
# build can serve at /api/v1 and at the bare root at the same time — so any
# value here could only be a guess. 3.1's default is a relative "/", which is
# the one entry that cannot lie.
#
# Unlike every other file this extension writes, this one has no phase-1 syntax
# gate: the standard library has no YAML parser, so a template bug reaches disk
# and is caught afterwards, by the fixture assertions and the e2e validator.
openapi: 3.1.0
info:
  title: "Todo API"
  version: "1.0.0"
paths:
  "/todos":
```

The `info.title` and `info.version` are the `WithOpenAPITitle` /
`WithOpenAPIVersion` options set in `todoent/entc.go`.

## Two things this example does that YOUR project should not copy

### 1. The generated directory is `todoent/`. Yours should be `ent/`.

Every ent project in the world puts its generated package in `ent/`, and so
should yours: `Target: "../ent"`, `Package: "your/module/ent"`, and the
generated package is then `package ent`, which is what every EntAPI doc and
every nested fixture module assumes when it aliases the import back to `ent`.

It is `todoent/` here for one reason that is specific to this repository: a
second `package ent` anywhere in the EntAPI tree makes the name ambiguous to
goimports, which resolves a bare `ent.` reference by package *name* across
everything it finds on the filesystem — ignoring module boundaries. That
ambiguity once rewrote `entgo.io/ent` into a fixture's import path inside a
generated `client.go` (#49), so `TestNoAmbiguousEntPackages` now fails the build
if any directory in this repository declares `package ent` — `examples/`
included. The `<name>ent` suffix is a workaround for hosting the example inside
the library's own repository, not a convention.

### 2. This module is separate, with a `replace` directive.

`examples/todo/go.mod` is its own module so it can depend on a SQL driver;
the EntAPI library module must stay driver-free. Its
`replace github.com/githonllc/entapi => ../..` points at the working tree so the
example always exercises the checked-out generator. In your project you would
just `require github.com/githonllc/entapi` at a released version and drop the
`replace`.

### Note for contributors

`make fmt` at the repository root runs `goimports -local github.com/githonllc/entapi`
over this directory (`FMT_FILES` excludes only `internal/fixture` and
`internal/fixtures`), and this module's own import path starts with that prefix.
The generated output committed here is therefore the post-`goimports` state. If
you re-run `go generate ./...`, run `make fmt` from the repository root before
checking `git status`, or the tree will look dirty.

## Where to look next

- The root [`README.md`](../../README.md) — the annotation model, the query wire
  format, and how to replace a generated operation with your own implementation
  via `With`.
- [`docs/GUIDE.md`](../../docs/GUIDE.md) — the same ground in full, with the
  reason behind each rule and a pointer to the source that makes it true:
  field shapes, the refusal matrix, the OpenAPI document, endpoint registration.
- `todoent/entapi_http.go` — `APIHandler`, `API`, `Mount`, `Endpoints`, and one
  `…Endpoint()` accessor per operation, for wrapping a single route in
  middleware.
- `todoent/todo_wiring.go` — the six exported wiring functions the handlers call.
  They take `*todoent.Client` and are usable straight from a service layer with
  no HTTP involved.
