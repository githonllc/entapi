package entdomain

import "testing"

// Regression tests for #5: the fluent builders use value receivers, so they look
// immutable, but the copy is shallow. Maps, slices and the Metadata pointer are
// shared with the receiver, so forking one base annotation into two chains lets
// each chain observe the other's writes, and a preset that hands out a
// package-level slice lets a caller corrupt that package-level value.

// restoreAllFieldScopes snapshots the package-level slice and puts its contents
// back afterwards, so a test that provokes global corruption cannot leak into
// the rest of the package's tests.
func restoreAllFieldScopes(t *testing.T) {
	t.Helper()
	saved := make([]FieldScope, len(AllFieldScopes))
	copy(saved, AllFieldScopes)
	t.Cleanup(func() {
		copy(AllFieldScopes, saved)
	})
}

func TestWithRequiredForkedChainsAreIndependent(t *testing.T) {
	base := DefaultField().WithRequired(ScopeCreate)
	a := base.WithRequired(ScopeUpdate)
	b := base.WithRequired(ScopeResponse)

	if a.Required[ScopeResponse] {
		t.Errorf("chain a leaked chain b's key: a.Required = %v", a.Required)
	}
	if b.Required[ScopeUpdate] {
		t.Errorf("chain b leaked chain a's key: b.Required = %v", b.Required)
	}
	if len(a.Required) != 2 || !a.Required[ScopeCreate] || !a.Required[ScopeUpdate] {
		t.Errorf("a.Required = %v, want exactly {create, update}", a.Required)
	}
	if len(b.Required) != 2 || !b.Required[ScopeCreate] || !b.Required[ScopeResponse] {
		t.Errorf("b.Required = %v, want exactly {create, response}", b.Required)
	}
	if len(base.Required) != 1 || !base.Required[ScopeCreate] {
		t.Errorf("base.Required = %v, want exactly {create}; the base must not see its forks' writes", base.Required)
	}
}

func TestDefaultFieldScopesDoNotAliasAllFieldScopes(t *testing.T) {
	restoreAllFieldScopes(t)

	d := DefaultField()
	d.Scopes[0] = ScopeQuery

	if AllFieldScopes[0] != ScopeCreate {
		t.Errorf("mutating DefaultField().Scopes[0] corrupted the package-level var: AllFieldScopes[0] = %v, want %v",
			AllFieldScopes[0], ScopeCreate)
	}

	other := DefaultField()
	if other.Scopes[0] != ScopeCreate {
		t.Errorf("a later DefaultField() caller saw the corruption: Scopes[0] = %v, want %v",
			other.Scopes[0], ScopeCreate)
	}
}

func TestDomainFieldWithScopesCopiesItsArgument(t *testing.T) {
	restoreAllFieldScopes(t)

	f := DomainFieldWithScopes(AllFieldScopes...)
	f.Scopes[0] = ScopeQuery

	if AllFieldScopes[0] != ScopeCreate {
		t.Errorf("mutating DomainFieldWithScopes(AllFieldScopes...).Scopes[0] corrupted the package-level var: AllFieldScopes[0] = %v, want %v",
			AllFieldScopes[0], ScopeCreate)
	}
}

func TestWithValidationCopiesItsArgument(t *testing.T) {
	rules := map[string]interface{}{"min": 1}
	f := DefaultField().WithValidation(rules)

	// The caller still holds the map it passed in. Writing to it must not
	// reach into the annotation that was already built from it.
	rules["min"] = 999
	rules["max"] = 5

	if got := f.Validation["min"]; got != 1 {
		t.Errorf("caller's later write reached the annotation: f.Validation[\"min\"] = %v, want 1", got)
	}
	if _, ok := f.Validation["max"]; ok {
		t.Errorf("caller's later insert reached the annotation: f.Validation = %v", f.Validation)
	}
}

func TestWithValidationKeepsNilNil(t *testing.T) {
	f := DefaultField().WithValidation(nil)
	if f.Validation != nil {
		t.Errorf("WithValidation(nil) produced a non-nil map %v, which would change the omitempty encoding", f.Validation)
	}
}

func TestMetadataForkedChainsAreIndependent(t *testing.T) {
	base := DefaultField().WithTitle("base")
	a := base.WithTitle("a")
	b := base.WithTitle("b")

	if a.Metadata == b.Metadata {
		t.Errorf("forked chains share one *FieldMetadata (%p)", a.Metadata)
	}
	if a.Metadata == base.Metadata {
		t.Errorf("fork a shares the base's *FieldMetadata (%p)", a.Metadata)
	}
	if a.Metadata.Title != "a" {
		t.Errorf("a.Metadata.Title = %q, want %q", a.Metadata.Title, "a")
	}
	if b.Metadata.Title != "b" {
		t.Errorf("b.Metadata.Title = %q, want %q", b.Metadata.Title, "b")
	}
	if base.Metadata.Title != "base" {
		t.Errorf("base.Metadata.Title = %q, want %q; the base must not see its forks' writes", base.Metadata.Title, "base")
	}
}

func TestMetadataForkedChainsCarryIndependentSlices(t *testing.T) {
	base := DefaultField().WithEnum("draft", "published").WithTags("core")
	a := base.WithTitle("a")

	a.Metadata.Enum[0] = "mutated"
	a.Metadata.Tags[0] = "mutated"

	if base.Metadata.Enum[0] != "draft" {
		t.Errorf("fork shares the base's Enum backing array: base.Metadata.Enum = %v", base.Metadata.Enum)
	}
	if base.Metadata.Tags[0] != "core" {
		t.Errorf("fork shares the base's Tags backing array: base.Metadata.Tags = %v", base.Metadata.Tags)
	}
}

// TestMetadataCopyPreservesEveryField guards the fix itself: copying the
// FieldMetadata pointee must carry every field across, not just the ones the
// next builder call happens to set.
func TestMetadataCopyPreservesEveryField(t *testing.T) {
	lo, hi := 1.5, 9.5
	minLen, maxLen := 2, 32

	base := DefaultField().
		WithTitle("title").
		WithFormat("email").
		WithPattern(`^\w+$`).
		WithRange(&lo, &hi).
		WithLength(&minLen, &maxLen).
		WithEnum("a", "b").
		WithTags("t1", "t2").
		AsReadOnly().
		AsWriteOnly().
		AsDeprecated()

	// WithTitle goes through the copy path; everything else must survive it.
	got := base.WithTitle("fork").Metadata

	if got.Title != "fork" {
		t.Errorf("got.Title = %q, want %q", got.Title, "fork")
	}
	if got.Format != "email" || got.Pattern != `^\w+$` {
		t.Errorf("string metadata lost across a copy: %+v", got)
	}
	if got.Minimum == nil || *got.Minimum != lo || got.Maximum == nil || *got.Maximum != hi {
		t.Errorf("numeric bounds lost across a copy: %+v", got)
	}
	if got.MinLength == nil || *got.MinLength != minLen || got.MaxLength == nil || *got.MaxLength != maxLen {
		t.Errorf("length bounds lost across a copy: %+v", got)
	}
	if len(got.Enum) != 2 || got.Enum[0] != "a" || got.Enum[1] != "b" {
		t.Errorf("Enum lost across a copy: %v", got.Enum)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "t1" || got.Tags[1] != "t2" {
		t.Errorf("Tags lost across a copy: %v", got.Tags)
	}
	if !got.ReadOnly || !got.WriteOnly || !got.Deprecated {
		t.Errorf("bool flags lost across a copy: %+v", got)
	}
}
