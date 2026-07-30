# EntDomain

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entdomain.svg)](https://pkg.go.dev/github.com/githonllc/entdomain)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entdomain)](https://goreportcard.com/report/github.com/githonllc/entdomain)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An [Ent](https://entgo.io) extension that generates HTTP request/response DTOs, base service structs, and base handler structs from annotated schemas.


> ### Status: prototype under redesign
>
> This library works, but its shape is being reconsidered. The direction is settled and
> **no part of it is implemented yet** — what is documented below is what exists today,
> not what is planned.
>
> - **Direction and rationale** — [`DESIGN-v2.md`](DESIGN-v2.md). It also records the
>   claims its own first draft got wrong, because knowing which intuitions fail in this
>   codebase is design material.
> - **Known defects** — [`QUALITY-REVIEW.md`](QUALITY-REVIEW.md), 41 findings from three
>   independent reviews.
> - **How it fits together** — [`ARCHITECTURE.md`](ARCHITECTURE.md).
> - **Work items** — epic [#23](https://github.com/githonllc/entdomain/issues/23).
>
> Read [Known limitations](#known-limitations) before adopting this. Some of them are
> traps rather than gaps, and one annotation documents a guarantee it does not provide.
>
> `go test ./...` is **red on a clean checkout** ([#2](https://github.com/githonllc/entdomain/issues/2)).

## Features

- **Annotation-driven** — mark field scopes with concise builders (`DefaultField`, `InputOnlyField`, `OutputOnlyField`, etc.)
- **HTTP DTOs** — generates `CreateRequest`, `UpdateRequest`, `Response`, `ListResponse` per entity
- **BaseService** — CRUD operations with Before/After hooks, builder helpers, and entity→response conversion
- **BaseHandler** — response conversion helpers and partial update support
- **Soft-delete detection** — automatically generates `UpdateOneID().SetDeletedAt(now)` for entities with a `deleted_at` field
- **Cursor pagination** — ID-based keyset pagination in BaseService
- **Source provenance** — generated files include schema name, template path, and regeneration command

## Requirements

- Go 1.23+
- [Ent](https://entgo.io) v0.14+

## Installation

```bash
go get github.com/githonllc/entdomain
```

## Setup

Wire the extension in your `entc.go`:

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/githonllc/entdomain"
)

func main() {
    ext := entdomain.NewExtensionWithOptions(
        entdomain.WithEntDomainPackage("github.com/githonllc/entdomain"),
        entdomain.WithBaseService(true),
        entdomain.WithBaseHandler(true),
    )

    if err := entc.Generate("./schema", &gen.Config{
        Target:  "../ent",
        Package: "your/module/ent",
    }, entc.Extensions(ext)); err != nil {
        log.Fatal(err)
    }
}
```

Then run:

```bash
go generate ./...
```

## Annotation Builders

### Base Builders

```go
entdomain.DefaultField()                      // create, update, query, response
                                              // also marks the field searchable, filterable and sortable
entdomain.InputOnlyField()                    // create + update only (e.g., password)
entdomain.OutputOnlyField()                   // response only (e.g., timestamps, state)
entdomain.CreateOnlyField()                   // create + response (immutable after creation)
entdomain.NewDomainField()                    // no scopes (tracked by ent but not in any HTTP struct)
entdomain.DomainFieldWithScopes(scopes...)    // custom scope combination
```

### Fluent Builder API

```go
field.String("email").
    Annotations(
        entdomain.DefaultField().
            WithRequired(entdomain.ScopeCreate),
    )
```

## Schema Example

```go
package schema

import (
    "time"

    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "github.com/githonllc/entdomain"
)

type User struct {
    ent.Schema
}

func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").
            NotEmpty().
            Annotations(
                entdomain.DefaultField().
                    WithRequired(entdomain.ScopeCreate),
            ),

        field.String("email").
            Optional().
            Annotations(entdomain.DefaultField()),

        field.Time("created_at").
            Default(time.Now).
            Immutable().
            Annotations(entdomain.OutputOnlyField()),
    }
}
```

## Architecture

```mermaid
graph TD
    subgraph "HTTP Layer"
        CR["CreateRequest<br/><small>ScopeCreate fields</small>"]
        UR["UpdateRequest<br/><small>ScopeUpdate fields</small>"]
        RS["Response<br/><small>ScopeResponse fields</small>"]
    end

    subgraph "ent/ package <small>(all generated)</small>"
        BH["BaseHandler<br/><small>ToResponse · ToResponseList · PartialUpdate</small>"]
        BS["BaseService<br/><small>Create · GetByID · Update · Delete<br/>ListWithCursor · DeleteBatch<br/>Before/After hooks</small>"]
        DTO["DTOs<br/><small>{entity}_dto.go</small>"]
    end

    EC["ent.Client<br/><small>Ent ORM</small>"]
    DB[("PostgreSQL")]

    CR --> BH
    UR --> BH
    BH --> RS
    BH --> BS
    BS --> EC
    EC --> DB
    DTO -.- BH
    DTO -.- BS

    style CR fill:#e1f5fe,stroke:#0288d1
    style UR fill:#e1f5fe,stroke:#0288d1
    style RS fill:#e8f5e9,stroke:#388e3c
    style BH fill:#fff3e0,stroke:#f57c00
    style BS fill:#fff3e0,stroke:#f57c00
    style DTO fill:#fff3e0,stroke:#f57c00
    style EC fill:#f3e5f5,stroke:#7b1fa2
    style DB fill:#fce4ec,stroke:#c62828
```

**Key principle**: Scopes only control HTTP-layer struct generation. The service layer operates directly on ent entities with full ORM capabilities.

## Generated Code

For each annotated schema, up to three files are generated (all in the `ent/` package):

| File | Contains |
|------|----------|
| `{entity}_dto.go` | `CreateRequest`, `UpdateRequest`, `Response`, `ListResponse`, `Validate()` methods |
| `{entity}_base_service.go` | `BaseService` with CRUD, Before/After hooks, `Apply*Request` builders, `EntToResponse` |
| `{entity}_base_handler.go` | `BaseHandler` with `ToResponse`, `ToResponseList`, `PartialUpdate` |

### BaseService Pattern

Generated `Base{Entity}Service` provides CRUD operations with hook extension points. Embed it and override hooks for custom logic:

```go
type myUserService struct {
    ent.BaseUserService
}

func NewMyUserService(db *ent.Client) *myUserService {
    s := &myUserService{
        BaseUserService: ent.BaseUserService{DB: db},
    }
    s.SetSelf(s) // enable hook dispatch to this struct
    return s
}

func (s *myUserService) BeforeCreate(ctx context.Context, req *ent.UserCreateRequest) error {
    // custom validation
    return nil
}

func (s *myUserService) AfterCreate(ctx context.Context, entity *ent.User) (*ent.User, error) {
    // publish event, etc.
    return entity, nil
}
```

## Typed Errors

BaseService wraps Ent errors with standard sentinel values:

```go
var (
    entdomain.ErrNotFound      // entity not found
    entdomain.ErrAlreadyExists // uniqueness constraint violation
    entdomain.ErrValidation    // validation failed
)
```

## Field Scopes

Scopes control which HTTP-layer DTOs include a field. They do **not** restrict service layer access.

| Scope | Description |
|-------|-------------|
| `ScopeCreate` | Field appears in `CreateRequest` |
| `ScopeUpdate` | Field appears in `UpdateRequest` |
| `ScopeResponse` | Field appears in `Response` |

## Extension Options

```go
entdomain.WithBaseService(true)              // generate BaseService (default: false)
entdomain.WithBaseHandler(true)              // generate BaseHandler (default: false)
entdomain.WithEntDomainPackage("custom/path") // override entdomain import path
```

## Known limitations

Verified against the source, not inferred from docs. Each links to the issue tracking it.

**An annotation that does not do what its name says.** `Sensitive` reads as a
data-protection marker. Nothing consults it — the response selector looks only at scopes,
so a field marked sensitive is emitted into responses like any other. Do not rely on it.
It is being removed rather than implemented: with a one-dimensional scope model, "never in
a response" is already expressible by omitting the response scope, so the annotation adds a
promise without adding a capability ([#3](https://github.com/githonllc/entdomain/issues/3)).

**Roughly twenty exported annotation fields are accepted, stored and ignored.** The API
accepts them without complaint, so there is no way to tell from the outside which ones do
anything. Only the scope list and the required map reach a template
([#17](https://github.com/githonllc/entdomain/issues/17)).

**`ScopeQuery` is granted by most preset builders and consumed by nothing.** It is
documented as placing a field in a query-parameter struct that no template emits.

**Every preset builder except `InputOnlyField` also marks the field searchable, filterable
and sortable.** Inert today. It matters for what comes next: sorting by an arbitrary column
is an unindexed-scan trigger and, combined with paging, an ordering oracle. When these
markers are implemented, defaulting them to on would make the allow-list meaningless
([#27](https://github.com/githonllc/entdomain/issues/27)).

**Soft delete silently disables downstream deletion hooks.** The generated delete is
rewritten as an update, which carries an update operation flag. A consumer hook registered
for the delete operations therefore never fires at all — this is not two mechanisms
conflicting, it is one silently replacing the other
([#12](https://github.com/githonllc/entdomain/issues/12)).

**The generated service supports one identifier type.** `uuid.UUID` is hardcoded in method
signatures. Non-UUID primary keys are unsupported
([#29](https://github.com/githonllc/entdomain/issues/29)).

**Hook dispatch fails silently when misused.** Forgetting the `SetSelf` call, or
misspelling a hook method, compiles cleanly and the hook never runs
([#16](https://github.com/githonllc/entdomain/issues/16)).

**Package import panics on Windows.** Template lookup joins paths with the OS separator
while the embedded filesystem always uses forward slashes, so loading fails at package
initialisation ([#4](https://github.com/githonllc/entdomain/issues/4)).

**Generated code is not compiled by any test in this repository.** Template changes are
effectively untested here; several field and edge shapes are known to produce output that
does not build ([#8](https://github.com/githonllc/entdomain/issues/8),
[#10](https://github.com/githonllc/entdomain/issues/10)).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

[MIT](LICENSE)
