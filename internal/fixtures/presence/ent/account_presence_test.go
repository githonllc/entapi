// This file is HAND-WRITTEN, like edges/ent/orerr_contract_test.go, and unlike
// everything else under a fixture's ent/ directory. It carries no DO NOT EDIT
// header on purpose.
//
// It is in package ent because the contract it pins is behavioural rather than
// syntactic: TestCodegenFixtures proves the generated create and patch requests
// COMPILE, which says nothing about whether an omitted field stays unwritten.
// The proof runs against real ent — real builders, real mutations — without a
// database: ent records every Set/Clear on the builder's mutation, and the
// mutation is exactly where "was this field written" is decided. This module
// deliberately has no SQL driver, so the mutation is the last observable point
// before the query ent would send.
package ent

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	entdomain "github.com/githonllc/entdomain/runtime"
	"github.com/google/uuid"
)

// decodeCreate unmarshals a create request the way a handler would, so presence
// comes from the JSON payload rather than from a struct literal.
func decodeCreate(t *testing.T, body string) *AccountCreateRequest {
	t.Helper()
	var r AccountCreateRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return &r
}

func decodePatch(t *testing.T, body string) *AccountPatchRequest {
	t.Helper()
	var r AccountPatchRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return &r
}

// validCreate is the minimum payload that passes validation: every required
// field present. Individual tests add or remove one key from it.
const validCreate = `{"email":"a@example.com","seats":3,"accepted_terms":true,"origin":"cli"}`

func mustValidateCreate(t *testing.T, body string) *ValidAccountCreateRequest {
	t.Helper()
	v, err := decodeCreate(t, body).Validate()
	if err != nil {
		t.Fatalf("validating %s: %v", body, err)
	}
	return v
}

// TestOmittedFieldOnCreateLeavesTheSchemaDefault is the first acceptance
// criterion of #14: a field that is neither required nor optional was written
// unconditionally, so omitting "plan" wrote the zero value and the schema's
// Default("free") never applied. The mutation is where that is visible.
func TestOmittedFieldOnCreateLeavesTheSchemaDefault(t *testing.T) {
	b := mustValidateCreate(t, validCreate).Apply(NewClient().Account.Create())

	if _, ok := b.Mutation().Plan(); ok {
		t.Error(`"plan" was omitted from the request but written to the mutation; ent's Default("free") can no longer apply`)
	}
	if _, ok := b.Mutation().Nickname(); ok {
		t.Error(`"nickname" was omitted but written to the mutation`)
	}
	if _, ok := b.Mutation().Quota(); ok {
		t.Error(`"quota" was omitted but written to the mutation`)
	}

	// The fields that WERE sent must reach the mutation, or the test above
	// would pass for a version of Apply that writes nothing at all.
	if got, ok := b.Mutation().Email(); !ok || got != "a@example.com" {
		t.Errorf(`"email" = (%q, %v), want ("a@example.com", true)`, got, ok)
	}
	if got, ok := b.Mutation().Seats(); !ok || got != 3 {
		t.Errorf(`"seats" = (%v, %v), want (3, true)`, got, ok)
	}
	if got, ok := b.Mutation().Origin(); !ok || got != "cli" {
		t.Errorf(`"origin" = (%q, %v), want ("cli", true)`, got, ok)
	}
}

// TestCreateSuppliedValueIsWrittenEvenWhenItIsTheZeroValue is the other half:
// presence, not the zero value, decides whether a field was supplied. A seat
// count of 0 and accepted_terms=false are values, not omissions.
func TestCreateSuppliedValueIsWrittenEvenWhenItIsTheZeroValue(t *testing.T) {
	body := `{"email":"a@example.com","seats":0,"accepted_terms":false,"origin":"cli","quota":0}`
	b := mustValidateCreate(t, body).Apply(NewClient().Account.Create())

	if got, ok := b.Mutation().Seats(); !ok || got != 0 {
		t.Errorf(`"seats" = (%v, %v), want (0, true): an explicit zero is a value`, got, ok)
	}
	if got, ok := b.Mutation().AcceptedTerms(); !ok || got {
		t.Errorf(`"accepted_terms" = (%v, %v), want (false, true)`, got, ok)
	}
	if got, ok := b.Mutation().Quota(); !ok || got != 0 {
		t.Errorf(`"quota" = (%v, %v), want (0, true)`, got, ok)
	}
}

// TestCreateRejectsAMissingRequiredField covers #14's "required-ness is checked
// for every required field regardless of Go type". v1 emitted a check only for
// required STRING fields, so a missing int or bool passed validation and failed
// later inside ent.
func TestCreateRejectsAMissingRequiredField(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"missing string", `{"seats":3,"accepted_terms":true,"origin":"cli"}`, "email"},
		{"missing int", `{"email":"a@example.com","accepted_terms":true,"origin":"cli"}`, "seats"},
		{"missing bool", `{"email":"a@example.com","seats":3,"origin":"cli"}`, "accepted_terms"},
		{"null int", `{"email":"a@example.com","seats":null,"accepted_terms":true,"origin":"cli"}`, "seats"},
		{"null string", `{"email":null,"seats":3,"accepted_terms":true,"origin":"cli"}`, "email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeCreate(t, tc.body).Validate()
			if err == nil {
				t.Fatalf("Validate() accepted %s, but %q is required", tc.body, tc.want)
			}
			if !errors.Is(err, entdomain.ErrValidation) {
				t.Errorf("error %v does not wrap entdomain.ErrValidation", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name the field %q", err, tc.want)
			}
		})
	}
}

// TestPatchDistinguishesAbsentNullAndValue is the whole point of the slice.
// With bare pointers, the first two rows are the same value.
func TestPatchDistinguishesAbsentNullAndValue(t *testing.T) {
	t.Run("absent leaves the field untouched", func(t *testing.T) {
		b := mustValidatePatch(t, `{}`).Apply(NewClient().Account.UpdateOneID(uuid.New()))
		if got := b.Mutation().Fields(); len(got) != 0 {
			t.Errorf("an empty patch wrote %v to the mutation; it must write nothing", got)
		}
		if b.Mutation().NicknameCleared() {
			t.Error("an empty patch cleared nickname")
		}
	})

	t.Run("explicit null clears an optional field", func(t *testing.T) {
		r := decodePatch(t, `{"nickname":null}`)
		if !r.HasNickname() {
			t.Fatal(`HasNickname() = false for {"nickname":null}: an explicit null is present`)
		}
		if r.Nickname != nil {
			t.Fatal("Nickname is non-nil for an explicit null")
		}
		b := mustValidatePatch(t, `{"nickname":null}`).Apply(NewClient().Account.UpdateOneID(uuid.New()))
		if !b.Mutation().NicknameCleared() {
			t.Error(`{"nickname":null} did not clear nickname; the field cannot be cleared at all`)
		}
	})

	t.Run("a value sets the field", func(t *testing.T) {
		b := mustValidatePatch(t, `{"nickname":"sam"}`).Apply(NewClient().Account.UpdateOneID(uuid.New()))
		if got, ok := b.Mutation().Nickname(); !ok || got != "sam" {
			t.Errorf(`"nickname" = (%q, %v), want ("sam", true)`, got, ok)
		}
		if b.Mutation().NicknameCleared() {
			t.Error("setting a value also cleared the field")
		}
	})
}

// TestPatchRejectsAnExplicitNullOnANonOptionalField: only fields the SCHEMA
// declares optional can be cleared, because ent emits Clear<Field>() for those
// and only those.
func TestPatchRejectsAnExplicitNullOnANonOptionalField(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"email":null}`, "email"},
		{`{"seats":null}`, "seats"},
		{`{"plan":null}`, "plan"},
	} {
		_, err := decodePatch(t, tc.body).Validate()
		if err == nil {
			t.Errorf("Validate() accepted %s, but %q is not optional and cannot be cleared", tc.body, tc.want)
			continue
		}
		if !errors.Is(err, entdomain.ErrValidation) {
			t.Errorf("error %v does not wrap entdomain.ErrValidation", err)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("error %q does not name the field %q", err, tc.want)
		}
	}
}

// TestPatchDropsAnImmutableFieldBeforeAnyValidatorRuns documents the one case
// #26 states is genuinely undetectable. "origin" is Immutable, so it has no
// place in the patch struct at all; encoding/json discards the key before
// Validate can see it. Rejecting it needs DisallowUnknownFields, which lives in
// the consumer's handler.
func TestPatchDropsAnImmutableFieldBeforeAnyValidatorRuns(t *testing.T) {
	if _, ok := reflect.TypeOf(AccountPatchRequest{}).FieldByName("Origin"); ok {
		t.Fatal("AccountPatchRequest carries the Immutable field Origin; ent generates no setter for it on AccountUpdateOne")
	}

	b := mustValidatePatch(t, `{"origin":"tampered"}`).Apply(NewClient().Account.UpdateOneID(uuid.New()))
	if got := b.Mutation().Fields(); len(got) != 0 {
		t.Errorf("a patch naming only an immutable field wrote %v", got)
	}
}

// TestApplyIsReachableOnlyThroughValidate is the compile-time half of the
// contract, asserted structurally because a test cannot contain code that does
// not compile. Apply lives on the validated type and nowhere else, so a caller
// who skips Validate has no method to call.
func TestApplyIsReachableOnlyThroughValidate(t *testing.T) {
	for _, tc := range []struct {
		raw, valid reflect.Type
	}{
		{reflect.TypeOf(&AccountCreateRequest{}), reflect.TypeOf(&ValidAccountCreateRequest{})},
		{reflect.TypeOf(&AccountPatchRequest{}), reflect.TypeOf(&ValidAccountPatchRequest{})},
	} {
		if _, ok := tc.raw.MethodByName("Apply"); ok {
			t.Errorf("%s has an Apply method: the builder is reachable without validating", tc.raw)
		}
		if _, ok := tc.valid.MethodByName("Apply"); !ok {
			t.Errorf("%s has no Apply method", tc.valid)
		}
	}
}

// TestRequestsMarshalAsOrdinaryJSON is the wire-format criterion. The presence
// map is unexported and carries no marshaller, so re-serialising a request has
// to produce the same document an ordinary struct would — this is the property
// the rejected Optional[T] wrapper lost.
func TestRequestsMarshalAsOrdinaryJSON(t *testing.T) {
	patch := decodePatch(t, `{"nickname":"sam","seats":4}`)
	out, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshalling the patch request: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("re-decoding %s: %v", out, err)
	}
	if len(round) != 2 || round["nickname"] != "sam" || round["seats"] != 4.0 {
		t.Errorf("patch marshalled as %s, want exactly the two keys it was decoded from", out)
	}

	create := decodeCreate(t, validCreate)
	out, err = json.Marshal(create)
	if err != nil {
		t.Fatalf("marshalling the create request: %v", err)
	}
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("re-decoding %s: %v", out, err)
	}
	if round["email"] != "a@example.com" || round["origin"] != "cli" {
		t.Errorf("create marshalled as %s, losing the values it was decoded from", out)
	}
}

// TestAHandBuiltRequestFallsBackToItsStruct pins the two asymmetric defaults for
// a request that never went through UnmarshalJSON, which is what a consumer's
// own service code and every table-driven test produces.
//
// A create request has no other source of truth than its fields, so they all
// count as supplied — rejecting "seats is required" for a struct that plainly
// sets Seats would be a lie. A patch request must default the other way: absent,
// so nothing is written. Reading a hand-built patch as "every field present"
// would turn its nil pointers into an instruction to clear the row.
func TestAHandBuiltRequestFallsBackToItsStruct(t *testing.T) {
	create := &AccountCreateRequest{Email: "a@example.com", Seats: 3, AcceptedTerms: true, Origin: "cli"}
	valid, err := create.Validate()
	if err != nil {
		t.Fatalf("a hand-built create request that sets every required field was rejected: %v", err)
	}
	b := valid.Apply(NewClient().Account.Create())
	if _, ok := b.Mutation().Plan(); ok {
		t.Error(`"plan" is nil on the request but was written to the mutation`)
	}

	patch := &AccountPatchRequest{Nickname: nil, Email: nil}
	validPatch, err := patch.Validate()
	if err != nil {
		t.Fatalf("validating a hand-built patch request: %v", err)
	}
	ub := validPatch.Apply(NewClient().Account.UpdateOneID(uuid.New()))
	if got := ub.Mutation().Fields(); len(got) != 0 {
		t.Errorf("a hand-built patch wrote %v; nil pointers there are not an instruction to clear", got)
	}
	if ub.Mutation().NicknameCleared() {
		t.Error("a hand-built patch cleared nickname")
	}
}

func mustValidatePatch(t *testing.T, body string) *ValidAccountPatchRequest {
	t.Helper()
	v, err := decodePatch(t, body).Validate()
	if err != nil {
		t.Fatalf("validating %s: %v", body, err)
	}
	return v
}
