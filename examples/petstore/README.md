# EntAPI petstore example

A runnable service with four entities: categories, pets, tags, and orders. The
seven files listed below are the example's hand-written input; everything below
the client, including DTOs, filters, wiring, HTTP handlers, the endpoint
manifest, soft-delete hooks, and `openapi.yaml`, is generated.

## Three commands

```bash
cd examples/petstore
go generate ./...
go run .
```

After regenerating, run `make fmt` at the repository root before committing.
`goimports` normalizes generated import grouping, and CI enforces that result.

The service listens on `http://localhost:8080`. A freshly started server prints
this manifest from `api.Endpoints()`:

```
2026/08/13 10:48:03 GET    /categories
2026/08/13 10:48:03 POST   /categories
2026/08/13 10:48:03 GET    /categories/{id}
2026/08/13 10:48:03 PATCH  /categories/{id}
2026/08/13 10:48:03 DELETE /categories/{id}
2026/08/13 10:48:03 GET    /orders
2026/08/13 10:48:03 POST   /orders
2026/08/13 10:48:03 GET    /orders/{id}
2026/08/13 10:48:03 PATCH  /orders/{id}
2026/08/13 10:48:03 GET    /pets
2026/08/13 10:48:03 POST   /pets
2026/08/13 10:48:03 GET    /pets/{id}
2026/08/13 10:48:03 PATCH  /pets/{id}
2026/08/13 10:48:03 DELETE /pets/{id}
2026/08/13 10:48:03 GET    /tags
2026/08/13 10:48:03 POST   /tags
2026/08/13 10:48:03 GET    /tags/{id}
2026/08/13 10:48:03 PATCH  /tags/{id}
2026/08/13 10:48:03 DELETE /tags/{id}
2026/08/13 10:48:03 GET    /openapi.yaml
2026/08/13 10:48:03 listening on http://localhost:8080
```

The database is in memory. Each run starts with two categories, three tags,
three pets, and two orders seeded by `main.go`, and nothing is written to disk.

## What you wrote, and what you got

You wrote seven files:

| File | What it is |
|---|---|
| `main.go` | opens an in-memory SQLite database, migrates and seeds it, builds `petstoreent.API(client)`, prints the generated manifest, and serves it |
| `petstoreent/schema/category.go` | the Category schema and its query annotations |
| `petstoreent/schema/pet.go` | the Pet schema, required category edge, expanded category and tags, and soft-delete mixin |
| `petstoreent/schema/tag.go` | the Tag schema and its deliberately smaller query surface |
| `petstoreent/schema/order.go` | the Order schema, required expanded pet edge, and excluded DELETE operation |
| `petstoreent/entc.go` | the `entc.Generate` call that installs EntAPI and sets the OpenAPI title and version |
| `petstoreent/generate.go` | the `//go:generate` directive that runs `entc.go`; without it, `go generate ./...` silently does nothing |

`api.Resource()` on each entity is the switch that produces its EntAPI files
and endpoints.

### Category

`name` has `api.Searchable()`, `api.Filterable()`, and `api.Sortable()`.
Searchable adds it to the `_q` search set, Filterable creates the per-field
`name` query parameter, and Sortable adds it to the `_sort` allow-list. The
inverse `pets` edge is useful to ent code but has no `api.Expand()`, so category
responses do not include pets.

### Pet

`name` is Searchable, Filterable, and Sortable. `status` and `price` are
Filterable and Sortable, while `created_at` is Sortable and ReadOnly.
`api.ReadOnly()` keeps `created_at` in responses but removes it from create and
patch requests.

The required category edge uses the explicit `category_id` field. A required
edge whose create family is reachable must name its backing field with
`edge.Field(...)`; that is what gives the generated create request a required
`category_id` field. `api.Expand()` on `category` and `tags` eager-loads those
edges and adds their summaries to pet responses.

`entapi.SoftDeleteMixin{}` changes Pet deletion into a tombstone write and
filters tombstoned pets from generated queries. The behavior is demonstrated
below.

### Tag

`name` is Filterable, so tags have a per-field `name` parameter. It is not
Searchable or Sortable, so it does not participate in `_q` and is not accepted
by `_sort`. The inverse `pets` edge is not expanded, so tag responses carry no
pets.

### Order

`status` is Filterable and Sortable. `created_at` is Sortable and ReadOnly, so
clients can sort by it and receive it, but cannot set or patch it. The required
pet edge has an explicit `pet_id` backing field for the reachable create family,
and `api.Expand()` adds a pet summary to order responses.

`api.Resource().Except(api.OpDelete)` removes both the `DELETE /orders/{id}`
route and the generated `DeleteOrderEndpoint` accessor. The lower-level
`DeleteOrder` wiring function remains available to Go code. Naming
`api.DeleteOrderEndpoint()` is a compile error, which prevents code from
assuming the excluded HTTP endpoint exists.

Expanded objects are summaries. Summaries carry no edges, so expansion is one
level deep: an order can include its pet, but that pet summary does not include
its category or tags.

## Exercising it

Every response below comes verbatim from one real run of these exact commands
against the freshly started server whose manifest appears above.

### List pets

The category and tags are nested summaries produced by `api.Expand()`.

```bash
curl -s localhost:8080/pets | python3 -m json.tool
```

```json
{
    "data": [
        {
            "id": 1,
            "name": "Buddy",
            "status": "available",
            "price": 499,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.878794-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        },
        {
            "id": 2,
            "name": "Luna",
            "status": "pending",
            "price": 350,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879035-07:00",
            "category_id": 2,
            "category": {
                "id": 2,
                "name": "Cats"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 2,
                    "name": "young"
                }
            ]
        },
        {
            "id": 3,
            "name": "Max",
            "status": "sold",
            "price": 275,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879101-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        }
    ],
    "total": 3,
    "page": 1,
    "size": 20
}
```

### Filter by status

```bash
curl -s 'localhost:8080/pets?status=available' | python3 -m json.tool
```

```json
{
    "data": [
        {
            "id": 1,
            "name": "Buddy",
            "status": "available",
            "price": 499,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.878794-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        }
    ],
    "total": 1,
    "page": 1,
    "size": 20
}
```

### Comparison operator spelling

The wire prefix is `ge`, not `gte`. With the default non-strict grammar, `gte`
is not recognized as a prefix. The parser falls back to whole-value equality,
then rejects `gte:300` because it cannot parse the whole string as a float.

```bash
curl -s 'localhost:8080/pets?price=gte:300' | python3 -m json.tool
```

```json
{
    "type": "about:blank",
    "title": "Bad Request",
    "status": 400,
    "detail": "validation failed: field \"price\" value \"gte:300\" is not a valid float64: strconv.ParseFloat: parsing \"gte:300\": invalid syntax"
}
```

Use `ge:300` for greater-than-or-equal:

```bash
curl -s 'localhost:8080/pets?price=ge:300' | python3 -m json.tool
```

```json
{
    "data": [
        {
            "id": 1,
            "name": "Buddy",
            "status": "available",
            "price": 499,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.878794-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        },
        {
            "id": 2,
            "name": "Luna",
            "status": "pending",
            "price": 350,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879035-07:00",
            "category_id": 2,
            "category": {
                "id": 2,
                "name": "Cats"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 2,
                    "name": "young"
                }
            ]
        }
    ],
    "total": 2,
    "page": 1,
    "size": 20
}
```

### Free-text search

`_q=bud` searches the Searchable `name` field.

```bash
curl -s 'localhost:8080/pets?_q=bud' | python3 -m json.tool
```

```json
{
    "data": [
        {
            "id": 1,
            "name": "Buddy",
            "status": "available",
            "price": 499,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.878794-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        }
    ],
    "total": 1,
    "page": 1,
    "size": 20
}
```

### Sort

`_sort=price:desc` is accepted because `price` is in the Sortable allow-list.

```bash
curl -s 'localhost:8080/pets?_sort=price:desc' | python3 -m json.tool
```

```json
{
    "data": [
        {
            "id": 1,
            "name": "Buddy",
            "status": "available",
            "price": 499,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.878794-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        },
        {
            "id": 2,
            "name": "Luna",
            "status": "pending",
            "price": 350,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879035-07:00",
            "category_id": 2,
            "category": {
                "id": 2,
                "name": "Cats"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 2,
                    "name": "young"
                }
            ]
        },
        {
            "id": 3,
            "name": "Max",
            "status": "sold",
            "price": 275,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879101-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        }
    ],
    "total": 3,
    "page": 1,
    "size": 20
}
```

### Invalid enum value

`status` is an enum, so a non-member is a 400 problem document.

```bash
curl -s -i 'localhost:8080/pets?status=flying' | head -8
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/problem+json
Date: Thu, 13 Aug 2026 17:48:05 GMT
Content-Length: 185

{"type":"about:blank","title":"Bad Request","status":400,"detail":"validation failed: field \"status\" value \"flying\" is not an enum member; legal members: available, pending, sold"}
```

### Create a category and a pet

```bash
curl -s -X POST localhost:8080/categories -H 'Content-Type: application/json' -d '{"name":"Birds"}' | python3 -m json.tool
```

```json
{
    "id": 3,
    "name": "Birds"
}
```

The Pet create request accepts `category_id` because the required edge names its
backing field explicitly.

```bash
curl -s -X POST localhost:8080/pets -H 'Content-Type: application/json' -d '{"name":"Kiwi","status":"available","price":120,"category_id":3}' | python3 -m json.tool
```

```json
{
    "id": 4,
    "name": "Kiwi",
    "status": "available",
    "price": 120,
    "photo_urls": null,
    "created_at": "2026-08-13T10:48:05.809734-07:00",
    "category_id": 3,
    "category": {
        "id": 3,
        "name": "Birds"
    },
    "tags": []
}
```

Omitting the required `category_id` is a 422:

```bash
curl -s -i -X POST localhost:8080/pets -H 'Content-Type: application/json' -d '{"name":"Ghost"}' | head -8
```

```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/problem+json
Date: Thu, 13 Aug 2026 17:48:05 GMT
Content-Length: 121

{"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"validation failed: category_id is required"}
```

### Patch a pet

```bash
curl -s -X PATCH localhost:8080/pets/2 -H 'Content-Type: application/json' -d '{"status":"sold"}' | python3 -m json.tool
```

```json
{
    "id": 2,
    "name": "Luna",
    "status": "sold",
    "price": 350,
    "photo_urls": null,
    "created_at": "2026-08-13T10:48:03.879035-07:00",
    "category_id": 2,
    "category": {
        "id": 2,
        "name": "Cats"
    },
    "tags": [
        {
            "id": 1,
            "name": "friendly"
        },
        {
            "id": 2,
            "name": "young"
        }
    ]
}
```

`created_at` is ReadOnly, so it is not a patch field. Sending it is the same as
sending any unknown field and returns 400:

```bash
curl -s -i -X PATCH localhost:8080/pets/2 -H 'Content-Type: application/json' -d '{"created_at":"2020-01-01T00:00:00Z"}' | head -8
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/problem+json
Date: Thu, 13 Aug 2026 17:48:05 GMT
Content-Length: 150

{"type":"about:blank","title":"Bad Request","status":400,"detail":"created_at: validation failed: unknown field \"created_at\"","field":"created_at"}
```

## Soft delete, demonstrated

Deleting Buddy returns 204 with no body:

```bash
curl -s -i -X DELETE localhost:8080/pets/1 | head -3
```

```http
HTTP/1.1 204 No Content
Date: Thu, 13 Aug 2026 17:48:05 GMT

```

The pet leaves generated lists:

```bash
curl -s localhost:8080/pets | python3 -m json.tool
```

```json
{
    "data": [
        {
            "id": 2,
            "name": "Luna",
            "status": "sold",
            "price": 350,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879035-07:00",
            "category_id": 2,
            "category": {
                "id": 2,
                "name": "Cats"
            },
            "tags": [
                {
                    "id": 1,
                    "name": "friendly"
                },
                {
                    "id": 2,
                    "name": "young"
                }
            ]
        },
        {
            "id": 3,
            "name": "Max",
            "status": "sold",
            "price": 275,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:03.879101-07:00",
            "category_id": 1,
            "category": {
                "id": 1,
                "name": "Dogs"
            },
            "tags": [
                {
                    "id": 3,
                    "name": "trained"
                }
            ]
        },
        {
            "id": 4,
            "name": "Kiwi",
            "status": "available",
            "price": 120,
            "photo_urls": null,
            "created_at": "2026-08-13T10:48:05.809734-07:00",
            "category_id": 3,
            "category": {
                "id": 3,
                "name": "Birds"
            },
            "tags": []
        }
    ],
    "total": 3,
    "page": 1,
    "size": 20
}
```

Soft delete does not cascade. The order keeps its `pet_id`, but expansion
filters the tombstoned target and returns `"pet": null`:

```bash
curl -s localhost:8080/orders/1 | python3 -m json.tool
```

```json
{
    "id": 1,
    "quantity": 1,
    "status": "placed",
    "ship_date": "2026-08-14T10:48:03.879153-07:00",
    "created_at": "2026-08-13T10:48:03.879169-07:00",
    "pet_id": 1,
    "pet": null
}
```

The deleted pet itself is no longer reachable:

```bash
curl -s -i localhost:8080/pets/1 | head -6
```

```http
HTTP/1.1 404 Not Found
Content-Type: application/problem+json
Date: Thu, 13 Aug 2026 17:48:05 GMT
Content-Length: 112

{"type":"about:blank","title":"Not Found","status":404,"detail":"entity not found: petstoreent: pet not found"}
```

The tombstone lives at ent's layer. Nothing in `main.go` knows about it: the
generated traverser filters reads, and the generated hook rewrites deletes.

## No DELETE /orders

`api.Resource().Except(api.OpDelete)` leaves the lower-level wiring available
but removes the HTTP operation. The standard library mux answers the attempted
method with 405:

```bash
curl -s -i -X DELETE localhost:8080/orders/1 | head -4
```

```http
HTTP/1.1 405 Method Not Allowed
Allow: GET, HEAD, PATCH
Content-Type: text/plain; charset=utf-8
X-Content-Type-Options: nosniff
```

## The query grammar

Field values split on the first colon. A bare value means `eq`; explicit wire
prefixes are `eq`, `ne`, `gt`, `ge`, `lt`, `le`, `in`, `not_in`, `from`, `to`,
and `between`. The spelling is `ge`, not `GTE`. Each field accepts only the
operators generated for its type and annotations.

Sort with `_sort=field:desc` or `_sort=field:asc`. Page with `_page` and `_size`.
Invalid values, enum members, operators, sort fields, and paging parameters
return 400 responses as `application/problem+json` documents.

## Where to look next

- The root [`README.md`](../../README.md) covers the annotation model, field
  shapes, query grammar, and operation replacement.
- `petstoreent/entapi_http.go` contains `APIHandler`, `API`, `Mount`,
  `Endpoints`, and the generated per-operation endpoint accessors.
- `petstoreent/openapi.yaml` is served at `GET /openapi.yaml`; its title is
  `Petstore API`.
