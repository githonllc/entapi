package entdomain

import (
	"fmt"

	"entgo.io/ent/entc/gen"
)

// setFieldCallReq generates a setter method call for a CreateRequest field (e.g., "SetName(req.Name)").
// For Nillable fields, uses SetNillable... to accept pointer types.
func setFieldCallReq(field *gen.Field, _ ...interface{}) string {
	if field.Nillable {
		return fmt.Sprintf("SetNillable%s(req.%s)", field.StructField(), field.StructField())
	}
	return fmt.Sprintf("Set%s(req.%s)", field.StructField(), field.StructField())
}
