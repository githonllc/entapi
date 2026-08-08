# 0009 — Errors are RFC 9457 problem+json; success responses are bare

**Status:** Accepted (2026-08-07, owner-adjudicated in the DESIGN-v3 grilling
session; classification wiring refined after review findings C3/C5) ·
**Date:** 2026-08-08 · **Tracking issue:** [#73](https://github.com/githonllc/entapi/issues/73),
[#74](https://github.com/githonllc/entapi/issues/74) · **Design:** `docs/DESIGN-v3-final.md` §2.3

## Context

A generated HTTP layer must fix an error envelope, and an envelope is a wire
contract that cannot be changed later without breaking clients. The project's
charter delegates governance to the ecosystem over inventing process
(IDEAL §5: protoc/buf precedent), which points the same way for wire
formats: adopt the standard rather than mint one.

## Decision

- **Errors:** `application/problem+json` (RFC 9457) with a `field` extension
  member for validation errors. Success responses stay bare: a single object
  is the DTO itself; a list is the `Page` shape
  `{"data","total","page","size"}` (connected to `{Entity}ListResponse`
  per #65).
- **Status table:** 201 Create · 200 Get/List/Patch · 204 Delete ·
  400 malformed JSON or illegal query parameter · 404 not-found ·
  409 unique violation · 422 validation (generated `Validate` failures and
  ent's `ValidationError` classified at Save) · 500 unclassified — including
  a duplicate key when no unique-violation probe is installed: a 500 on a
  duplicate is recoverable, a 409 on a foreign-key failure is a wrong answer
  (inherited from `errors.tmpl`).
- **Strict bodies:** generated handlers set `DisallowUnknownFields`; an
  unknown or immutable key in a body is a 400 naming the key, never a silent
  drop (ADR-0001's direction applied to the body).
- **Classification wiring:** generated code hands the runtime three
  predicates (`IsNotFound`, `IsConstraintError`, `IsValidationError`) plus a
  field-name extractor `func(error) (string, bool)` built on
  `errors.As(*ent.ValidationError)` — a bare bool cannot carry
  `ValidationError.Name`, and the runtime must not know ent types.

## Consequences

- Gateways, middleware and frontend libraries understand the envelope
  without bespoke parsing; future multi-error reporting has a standard
  extension path (`errors` array) if ever needed — today ent's generated
  `check()` fails one field at a time, so the singular `field` member is
  sufficient by construction.
- Known residue: validation failures that ent reports as bare errors
  (e.g. clearing and setting a required edge in one mutation) land as 500,
  not 422 — accepted, consistent with the "never misclassify" direction.
- Validation happens at Save time, not at bind time; numeric/pattern
  constraints do not appear in OpenAPI (ADR-0006's validator decision).

## Alternatives considered

- **Minimal custom envelope** (`{"error","field"}`) — shorter, but a
  home-grown frozen wire contract; rejected.
- **Rich nested envelope** (`{"error":{code,message,fields}}`) — neither
  standard nor minimal; rejected.
