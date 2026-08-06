package fixture

import (
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

// TestEdgeAnnotationsSurviveToCodegen guards a trap that is otherwise silent.
//
// Declaring a self-referential pair in the chained form
//
//	edge.To("children", X.Type).From("parent").Unique().Field("parent_id").Annotations(a)
//
// attaches the annotation to the INVERSE edge only. The assoc edge is left
// unannotated, no error is reported, and the edge simply never appears in a
// response. The schema declares the two edges separately for this reason.
func TestEdgeAnnotationsSurviveToCodegen(t *testing.T) {
	g, err := entc.LoadGraph("./spikeent/schema", &gen.Config{
		Package: "github.com/githonllc/entdomain/internal/fixture/spikeent",
	})
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}

	want := map[string]bool{
		"User.posts":        true,
		"Post.author":       true,
		"Category.children": true,
		"Category.parent":   true,
	}

	seen := map[string]bool{}
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			key := n.Name + "." + e.Name
			seen[key] = true
			_, ok := e.Annotations["DomainEdge"]
			if want[key] && !ok {
				t.Errorf("%s: DomainEdge annotation did not reach codegen", key)
			}
		}
	}
	for key := range want {
		if !seen[key] {
			t.Errorf("%s: edge missing from the graph entirely", key)
		}
	}
}

// TestEdgeAnnotationArrivesAsMap records the representation the reader must
// handle. Annotations do not arrive as the Go type they were written as.
func TestEdgeAnnotationArrivesAsMap(t *testing.T) {
	g, err := entc.LoadGraph("./spikeent/schema", &gen.Config{
		Package: "github.com/githonllc/entdomain/internal/fixture/spikeent",
	})
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			raw, ok := e.Annotations["DomainEdge"]
			if !ok {
				continue
			}
			if _, isMap := raw.(map[string]interface{}); !isMap {
				t.Fatalf("%s.%s: expected map[string]interface{}, got %T — "+
					"the JSON round-trip in getDomainEdgeAnnotation exists for this",
					n.Name, e.Name, raw)
			}
			return
		}
	}
	t.Fatal("no annotated edge found")
}
