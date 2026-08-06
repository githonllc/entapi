# 0001 — Request key matching is strict: a case-variant key is an error, never a silent no-op

**Status:** Accepted (2026-08-06, owner-delegated arbitration — panel `[degraded]`:
Gemini 3.1 Pro + Fable, unanimous; deciding fact: the silent-loss path means a
"working" case-variant client cannot exist, so rejection breaks nobody) ·
**Date:** 2026-08-06 · **Tracking issue:** [#58](https://github.com/githonllc/entdomain/issues/58)

**Ratification constraint:** the rejection's fold must remain a **superset** of
the decoder's field matching, or the silent path reopens for exotic folds.
`strings.EqualFold` (Unicode simple folding, which includes the Kelvin-sign
class) satisfies this for every historical variant of `encoding/json`'s
matching; anything narrower than the decoder's fold voids rationale 3.

## Context

The generated request types record *presence* by raw payload key
(`templates/dto.tmpl`, both `UnmarshalJSON`s), while `encoding/json` matches
struct fields **case-insensitively** ("preferring an exact match but also
accepting a case-insensitive match"). The two disagree about what a payload
carried whenever a key is a case variant of the canonical tag:

- `PATCH {"Name":"x"}` against tag `name`: the struct field decodes to `"x"`,
  `HasName()` is false, `Apply` writes nothing — **the update reports success
  and changes nothing**.
- `POST` with a case-variant key on an *optional* field: same silent drop.
  Only *required* create fields fail closed (the presence check rejects).
- Case-variant duplicates (`{"name":"a","Name":null}`) can clear a clearable
  patch field even though the exact-tag key carried a value: `encoding/json`
  lets the later key overwrite the field, while `present` sees both raw keys.

Every existing presence test uses exact lower-case keys
(`internal/fixtures/presence/presenceent/account_presence_test.go`), which is
why this survived.

## Decision

The generated `UnmarshalJSON` **rejects** any payload key that case-folds to a
known field tag without matching it exactly, wrapping `ErrValidation` and
naming both the offending key and the canonical tag. Unknown keys that fold to
no tag keep their current behaviour (ignored — rejecting those is
`DisallowUnknownFields`, which stays the consumer handler's decision, as
already documented for immutable fields).

Rejection is chosen over fold-matching presence (mirroring `encoding/json`)
because:

1. The presence model is the load-bearing idea of the request layer — three
   states, never silence. Fold-matching keeps a class of ambiguity alive
   (duplicate case-variant keys have order-dependent meaning that the
   `map[string]json.RawMessage` intermediate cannot even observe).
2. A client sending `"Name"` for `"name"` is broken today — it silently loses
   data. A 400 naming the key is the first honest answer it ever gets.
3. It is the only option whose implementation cannot disagree with
   `encoding/json` in some further corner, because it removes the corner.

## Consequences

- Wire behaviour changes: payloads that previously "succeeded" (by silently
  dropping a field) now fail with a named validation error. This is a
  bug-compatible break, documented in both READMEs' migration notes.
- One new loop in each generated `UnmarshalJSON`, over raw keys × a generated
  canonical-tag set. No runtime-package change.
- Presence tests gain case-variant and duplicate-key rows.

## Alternatives considered

- **Fold-match presence, reject only fold-duplicates** — mirrors
  `encoding/json` most closely, but keeps silent acceptance of sloppy clients
  and needs order information JSON maps do not preserve.
- **Do nothing, document the strictness gap** — leaves a verified silent
  data-loss path (the PATCH no-op) in every consumer.
