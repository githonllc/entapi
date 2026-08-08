# 0002 — Every generated list order ends with a primary-key tiebreak

**Status:** Accepted (2026-08-06, owner-delegated arbitration — panel `[degraded]`:
Gemini 3.1 Pro + Fable, unanimous; deciding fact: ent emits `ByID` for every
fixture key shape, and offset paging without a total order loses rows with zero
concurrent writes) · **Date:** 2026-08-06 · **Tracking issue:** [#59](https://github.com/githonllc/entdomain/issues/59)

**Ratification constraint:** when the requested sort key *is* the primary key,
the tiebreak is skipped — never `ORDER BY id, id`.

> **Addendum (2026-08-08, second adversarial round — v3 §12 A4):** DESIGN-v3
> makes `_sort` a multi-key list, so the constraint generalizes: the tiebreak
> is skipped whenever the primary key appears at **any position** in the
> requested sort list — never `ORDER BY id, …, id`. The single-key wording
> above predates the list form; the generalized form governs.

## Context

`{Entity}Order` (`templates/filter.tmpl`) returns `nil, nil` when the request
names no sort key, and `ListPage` (`runtime/query.go`) then issues
`LIMIT/OFFSET` with **no ORDER BY**. SQL row order without ORDER BY is
unspecified: rows can repeat or vanish across pages with zero concurrent
writes. The same hazard exists *with* a requested sort whenever the sort
column is non-unique — equal keys have unspecified relative order, so page
boundaries cut through them nondeterministically.

The existing "no default sort key" policy (`filter.tmpl`) is about which
*column* to prefer when the caller names none — a policy the schema genuinely
does not contain. Determinism is a different question: it has a schema-given
answer (the primary key), requires no policy invention, and offset pagination
is simply incorrect without it.

## Decision

`{Entity}Order` always appends the entity's primary-key order as the final
order term:

- Caller requested a sort: `[]OrderOption{by(dir), ByID(dir)}` — the tiebreak
  follows the requested direction, so a fully descending walk stays fully
  descending.
- Caller requested nothing: `[]OrderOption{ByID(asc)}` — deterministic, and
  deliberately *not* presented as a "default sort" in the API: the response
  does not claim an ordering the caller never asked for; it merely stops being
  random.

The tiebreak is emitted by the template, not the runtime: `ListPage` keeps
taking resolved order options, and the runtime stays free of entity knowledge.

## Consequences

- Every generated list query gains an ORDER BY — a real but small cost, paid
  for correctness that offset pagination requires anyway.
- Generated output changes for every consumer; fixtures regenerate; the wiring
  e2e module gains a two-page walk asserting no row repeats or vanishes.
- Implementation must confirm ent emits `ByID` for every entity (it does for
  the current fixture set; verify against ent's meta template before relying
  on it for custom-named IDs).

## Alternatives considered

- **Reject unsorted list requests** — punishes the common case to preserve a
  purity argument; the PK order is not a guess, so refusing is not honesty.
- **Tiebreak in `ListPage`** — the runtime would need the ID order option per
  entity, importing exactly the knowledge the package split exists to keep
  out.
- **Document the nondeterminism** — "pages may lose rows" is not a documentable
  behaviour for a pagination API; it is the absence of one.
