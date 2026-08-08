# 0007 — Query wire format: op-in-value with an underscore-reserved namespace

**Status:** Accepted (2026-08-07/08, owner-adjudicated; parse rules refined
after cross-family review findings A1/A2) · **Date:** 2026-08-08 ·
**Tracking issue:** — (assigned when the implementation arc opens) ·
**Design:** `docs/DESIGN-v3-final.md` §3

## Context

Filter parameters need a wire syntax. The generated `{Entity}Filter` structs
(#27) carried `form` tags spelling one parameter per operator
(`title_neq`, `score_gt`). The owner's prior library entigo
(`~/workspace/githon/entigo`) established an op-in-value convention
(`age=gt:30`, split on the first colon) that is production-proven and compact:
one parameter per field. With bare field names as parameters, `sort`/`page`/
`size` collide with same-named columns — a real entity with a `size` column
makes `?size=gt:30` ambiguous.

## Decision

- **Wire syntax: op-in-value.** `field=op:value`, split on the first colon.
  Operator vocabulary: bare (eq), `ne:`, `gt:/ge:/lt:/le:`, `in:/not_in:`
  (comma-separated), `like:/ilike:/prefix:/suffix:` (substring class, gated
  per ADR-0005), `is_null:/not_null:`, `from:/to:/between:` (gte+lte sugar).
- **Value parsing, first match wins:** (1) bare empty value → filter ignored;
  explicit `eq:` → match empty string. (2) no colon → eq literal. (3) prefix
  in this field's allowed set → operator. (4) prefix is a globally known
  operator this field does not allow → **400** (the ADR-0005 gate stays
  loud). (5) unknown prefix → whole value falls back to an eq literal
  (naive `12:30`/URL values just work). (6) explicit `eq:` is the literal
  escape hatch.
- **Per-field operator sets are computed at codegen** ($field.Ops ∩ markers)
  and emitted as a parse switch — no runtime operator table. Parsing fills
  the retained typed `{Entity}Filter`; wiring and service-layer APIs are
  unchanged.
- **Reserved namespace:** parameters starting with `_` belong to the
  framework; bare names belong to the schema. Exactly four, no aliases:
  `_sort=field:desc,f2` (multi-field, Sortable allow-list, invalid field →
  400, PK tiebreak appended per ADR-0002), `_page`/`_size` (1-indexed,
  default 20, cap 1000), `_q` (Contains-OR over Searchable fields).
  A schema field name starting with `_` that reaches the query surface is
  refused at generation.

## Consequences

- OpenAPI filter parameters degrade to `type: string` with pattern +
  description (accepted cost); frontend types lose per-operator typing.
- Commas inside `in:`/`between:` values are inexpressible — percent-encoding
  cannot help because `r.URL.Query()` decodes before the parser runs.
- Typos of operator prefixes on string fields silently become literals
  (rule 5); typed fields still fail value-parse to 400.
- `form` tags and the `sort_by`/`order`/`q` parameter pair retire;
  `ListRequest` v2 carries a sort-spec list.

## Alternatives considered

- **One parameter per operator (form-tag contract)** — full type chain into
  OpenAPI and structurally impossible illegal operators, but verbose and
  discontinuous with the owner's entigo lineage; overruled.
- **Bare reserved names with collision refusal** — pushes renames onto
  common column names; rejected.
- **Reserved aliases (`_size` and `size` both accepted)** — ambiguity
  survives on colliding entities and parsing becomes schema-dependent;
  rejected.
