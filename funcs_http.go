package entapi

import (
	"fmt"
	"sort"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// resourceOperation is the complete routing identity of one public CRUD
// operation. Keeping it data-shaped lets both HTTP templates consume the same
// Except decision without maintaining parallel switches.
type resourceOperation struct {
	Name       api.Op
	Method     string
	PathSuffix string
	Function   string
	Field      string
	Handler    string
}

// resourceOps returns the public operations generated for node in route order.
func resourceOps(node *gen.Type) []resourceOperation {
	if node == nil || !isResource(node) {
		return nil
	}
	n := node.Name
	p := entPlural(n)
	candidates := []resourceOperation{
		{Name: api.OpList, Method: "GET", Function: "List" + p},
		{Name: api.OpCreate, Method: "POST", Function: "Create" + n},
		{Name: api.OpGet, Method: "GET", PathSuffix: "/{id}", Function: "Get" + n},
		{Name: api.OpPatch, Method: "PATCH", PathSuffix: "/{id}", Function: "Patch" + n},
		{Name: api.OpDelete, Method: "DELETE", PathSuffix: "/{id}", Function: "Delete" + n},
	}
	out := make([]resourceOperation, 0, len(candidates))
	for _, op := range candidates {
		if resourceExcepts(node, op.Name) || op.Name == api.OpCreate && !hasCreateFamily(node) {
			continue
		}
		op.Field = camelCase(op.Function)
		op.Handler = "handle" + op.Function
		out = append(out, op)
	}
	return out
}

// routePath returns the collection path using Ent's own plural and snake
// functions, so irregular entity names follow the same vocabulary as Ent.
func routePath(node *gen.Type) string {
	if node == nil {
		return ""
	}
	return "/" + entSnake(entPlural(node.Name))
}

var entSnake = mustEntSnake()

func mustEntSnake() func(string) string {
	fn, ok := gen.Funcs["snake"].(func(string) string)
	if !ok {
		panic("entapi: entc/gen no longer exposes \"snake\" as func(string) string")
	}
	return fn
}

// idParseExpr returns the request-time expression for parsing a path id.
func idParseExpr(id *gen.Field) string {
	if id == nil || id.Type == nil {
		return ""
	}
	switch {
	case id.Type.Type == field.TypeUUID:
		return `uuid.Parse(r.PathValue("id"))`
	case id.Type.Type.Integer():
		_, bits, _ := fieldParseConversion(id)
		return fmt.Sprintf(`strconv.ParseInt(r.PathValue("id"), 10, %d)`, bits)
	case id.Type.Type == field.TypeString:
		return `r.PathValue("id")`
	default:
		return `r.PathValue("id")`
	}
}

// handlerImports returns exactly the imports used by one generated handler
// file for its reachable operations and identifier type.
func handlerImports(node *gen.Type) []string {
	ops := resourceOps(node)
	if len(ops) == 0 {
		return nil
	}
	specs := map[string]bool{`"context"`: true, `"net/http"`: true}
	var hasBody, hasJSON, hasID bool
	for _, op := range ops {
		switch op.Name {
		case api.OpCreate, api.OpPatch:
			hasBody = true
			hasJSON = true
		case api.OpList, api.OpGet:
			hasJSON = true
		}
		hasID = hasID || op.PathSuffix != ""
	}
	if hasJSON {
		specs[`"encoding/json"`] = true
	}
	if hasBody {
		for _, spec := range []string{`"errors"`, `"fmt"`, `"io"`, `"mime"`} {
			specs[spec] = true
		}
	}
	if hasID && node.ID != nil && node.ID.Type != nil {
		switch {
		case node.ID.Type.Type == field.TypeUUID:
			specs[`"fmt"`] = true
			specs[`"github.com/google/uuid"`] = true
		case node.ID.Type.Type.Integer():
			specs[`"fmt"`] = true
			specs[`"strconv"`] = true
		}
	}
	out := make([]string, 0, len(specs))
	for spec := range specs {
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}
