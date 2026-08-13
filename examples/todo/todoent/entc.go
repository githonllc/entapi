//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/githonllc/entapi"
)

func main() {
	ext := entapi.NewExtensionWithOptions(
		entapi.WithOpenAPITitle("Todo API"),
		entapi.WithOpenAPIVersion("1.0.0"),
	)

	if err := entc.Generate("./schema", &gen.Config{
		Target:  "../todoent",
		Package: "github.com/githonllc/entapi/examples/todo/todoent",
	}, entc.Extensions(ext)); err != nil {
		log.Fatal(err)
	}
}
