package entapi

import (
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"

	"github.com/githonllc/entapi/api"
)

func fieldNames(fields []*gen.Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

func TestFieldSelectorsDeriveFromEntAndDeviationWords(t *testing.T) {
	plain := newStringField("plain", nil)
	hidden := newStringField("hidden", fieldPtr(api.Hidden()))
	readOnly := newStringField("read_only", fieldPtr(api.ReadOnly()))
	immutable := newStringField("immutable", nil)
	immutable.Immutable = true
	node := newTestType("Widget", plain, hidden, readOnly, immutable)

	if got, want := fieldNames(createFields(node)), []string{"plain", "immutable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("createFields = %v, want %v", got, want)
	}
	if got, want := fieldNames(patchFields(node)), []string{"plain"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("patchFields = %v, want %v", got, want)
	}
	if got, want := fieldNames(responseFields(node)), []string{"plain", "read_only", "immutable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("responseFields = %v, want %v", got, want)
	}
}

func TestSensitiveRemainsSettable(t *testing.T) {
	// gen.Field.def is unexported, so a unit test cannot manufacture a truly
	// Sensitive field. This assertion pins the selector direction without
	// pretending to prove response exclusion; the sensitive fixture does that.
	secret := newStringField("secret", nil)
	node := newTestType("Session", secret)
	if got := createFields(node); len(got) != 1 || got[0] != secret {
		t.Fatalf("createFields = %v, want secret", fieldNames(got))
	}
	if got := patchFields(node); len(got) != 1 || got[0] != secret {
		t.Fatalf("patchFields = %v, want secret", fieldNames(got))
	}
}

func TestResponseEdgesUseExpandAndRequireAResourceTarget(t *testing.T) {
	target := newTestType("User", newStringField("name", nil))
	plain := &gen.Edge{Name: "plain", Type: target}
	expanded := &gen.Edge{
		Name:        "author",
		Type:        target,
		Annotations: gen.Annotations{edgeAnnotationName: edgePtr(api.Expand().JSONKey("owner"))},
	}
	node := newTestType("Post", newStringField("title", nil))
	node.Edges = []*gen.Edge{plain, expanded}

	got, err := responseEdges(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != expanded {
		t.Fatalf("responseEdges = %v, want author", got)
	}
	if key := edgeJSONKey(expanded); key != "owner" {
		t.Fatalf("edgeJSONKey = %q, want owner", key)
	}
	if key := edgeJSONKey(plain); key != "plain" {
		t.Fatalf("default edgeJSONKey = %q, want plain", key)
	}

	target.Annotations = nil
	_, err = responseEdges(node)
	if err == nil || !strings.Contains(err.Error(), "not an api.Resource") {
		t.Fatalf("responseEdges error = %v, want non-Resource refusal", err)
	}
}
