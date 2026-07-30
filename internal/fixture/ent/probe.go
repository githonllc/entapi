//go:build ignore

package main

import (
	"fmt"
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	g, err := entc.LoadGraph("./schema", &gen.Config{
		Package: "github.com/githonllc/entdomain/internal/fixture/ent",
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, n := range g.Nodes {
		for _, e := range n.Edges {
			ann, ok := e.Annotations["DomainEdge"]
			fmt.Printf("%-9s .%-9s unique=%-5v inverse=%-5v edge.Field()!=nil=%-5v  DomainEdge=%v %T\n",
				n.Name, e.Name, e.Unique, e.IsInverse(), e.Field() != nil, ok, ann)
		}
	}
}
