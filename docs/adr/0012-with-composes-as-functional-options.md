# 0012 — `With` composes as functional options: last-wins, nil panics

**Status:** Accepted (2026-08-08, owner decision; recorded as its own ADR by
owner call after a cross-family arbitration split on placement — v3 §12.5) ·
**Date:** 2026-08-08 ·
**Tracking issue:** [#75](https://github.com/githonllc/entapi/issues/75) ·
**Design:** `docs/DESIGN-v3-final.md` §2.2

## Context

`ent.API(client)` exposes one customization mechanism: `With(...)` replaces a
generated operation's default function with the consumer's, at the wiring
signature (ADR-0008). What was left open was the *combination* semantics —
what repeated, chained, or invalid options mean. Those semantics are a wire
of their own: consumers will build conditional assembly on them, and changing
them later breaks call sites silently.

## Decision

`With` follows Go's functional-options convention:

- **Variadic ≡ chained:** `With(a, b).With(c)` ≡ `With(a, b, c)`.
- **Last-wins:** setting the same customization point twice keeps the later
  value — supporting the idiomatic "defaults first, conditional override
  after" assembly.
- **nil rejected at construction, by panic:** a nil function value can never
  be intended; falling back silently to the default is exactly the class of
  silence ADR-0001 exists to kill. The chained signature has no error
  channel, and a programmer error at wiring time panics by stdlib convention
  (`http.Handle` on a nil handler is the same shape).

## Consequences

- Conditional assembly is first-class; no `Reset`/`Remove` API is needed.
- A nil customization fails at startup, not at first request.
- The semantics are a compatibility contract from the first release —
  last-wins can never quietly become first-wins or reject-duplicates.

## Alternatives considered

- **Error-returning `With`** — breaks chaining; construction errors here are
  programmer errors, not runtime conditions.
- **Reject duplicate settings** — kills the defaults-then-override idiom for
  no safety gain.
- **Silent fallback on nil** — the ADR-0001 anti-pattern.
