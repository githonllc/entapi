package entdomain

import (
	"fmt"
	"strings"

	"entgo.io/ent/entc/gen"
)

// getEntityPackageName returns the correct ent package name for an entity
func getEntityPackageName(node *gen.Type) string {
	// The ent-generated package name is the lowercase form of the entity name
	return strings.ToLower(node.Name)
}

// generateSearchCondition generates search condition for searchable string fields
func generateSearchCondition(field *gen.Field, node *gen.Type) string {
	if field.Type.String() == "string" {
		packageName := getEntityPackageName(node)
		return fmt.Sprintf("		predicates = append(predicates, %s.%sContains(req.Query))", packageName, field.StructField())
	}
	return ""
}

// generateIdOperation generates ID-related operations for the given type
func generateIdOperation(node *gen.Type, operation string, idVar string) string {
	idType := node.ID.Type.String()
	entityName := node.Name
	packageName := getEntityPackageName(node)

	switch operation {
	case "get":
		if idType == "string" {
			return fmt.Sprintf(`entity, err := r.client.%s.Get(ctx, %s.String())`, entityName, idVar)
		} else if idType == "int64" {
			return fmt.Sprintf(`entity, err := r.client.%s.Get(ctx, func() int64 {
		if i, err := %s.Int64(); err == nil {
			return i
		}
		return 0
	}())`, entityName, idVar)
		} else if isUUIDType(idType) {
			return fmt.Sprintf(`uid, parseErr := uuid.Parse(%s.String())
	if parseErr != nil {
		return nil, fmt.Errorf("invalid uuid: %%w", parseErr)
	}
	entity, err := r.client.%s.Get(ctx, uid)`, idVar, entityName)
		} else {
			return fmt.Sprintf(`entity, err := r.client.%s.Get(ctx, %s)`, entityName, idVar)
		}
	case "updateOneID":
		return fmt.Sprintf(`entity, err := r.client.%s.UpdateOneID(model.%s).`, entityName, node.ID.StructField())
	case "delete":
		if idType == "string" {
			return fmt.Sprintf(`return r.client.%s.DeleteOneID(%s.String()).Exec(ctx)`, entityName, idVar)
		} else if idType == "int64" {
			return fmt.Sprintf(`return r.client.%s.DeleteOneID(func() int64 {
		if i, err := %s.Int64(); err == nil {
			return i
		}
		return 0
	}()).Exec(ctx)`, entityName, idVar)
		} else if isUUIDType(idType) {
			return fmt.Sprintf(`uid, parseErr := uuid.Parse(%s.String())
	if parseErr != nil {
		return fmt.Errorf("invalid uuid: %%w", parseErr)
	}
	return r.client.%s.DeleteOneID(uid).Exec(ctx)`, idVar, entityName)
		} else {
			return fmt.Sprintf(`return r.client.%s.DeleteOneID(%s).Exec(ctx)`, entityName, idVar)
		}
	case "softDelete":
		if isUUIDType(idType) {
			return fmt.Sprintf(`uid, parseErr := uuid.Parse(%s.String())
	if parseErr != nil {
		return fmt.Errorf("invalid uuid: %%w", parseErr)
	}
	return r.client.%s.UpdateOneID(uid).SetDeletedAt(time.Now()).Exec(ctx)`, idVar, entityName)
		} else if idType == "string" {
			return fmt.Sprintf(`return r.client.%s.UpdateOneID(%s.String()).SetDeletedAt(time.Now()).Exec(ctx)`, entityName, idVar)
		} else if idType == "int64" {
			return fmt.Sprintf(`idVal, parseErr := %s.Int64()
	if parseErr != nil {
		return fmt.Errorf("invalid id: %%w", parseErr)
	}
	return r.client.%s.UpdateOneID(idVal).SetDeletedAt(time.Now()).Exec(ctx)`, idVar, entityName)
		} else {
			return fmt.Sprintf(`return r.client.%s.UpdateOneID(%s).SetDeletedAt(time.Now()).Exec(ctx)`, entityName, idVar)
		}
	case "softDeleteBatch":
		if isUUIDType(idType) {
			return fmt.Sprintf(`now := time.Now()
	uids := make([]uuid.UUID, len(%s))
	for i, id := range %s {
		uid, parseErr := uuid.Parse(id.String())
		if parseErr != nil {
			return fmt.Errorf("invalid uuid at index %%d: %%w", i, parseErr)
		}
		uids[i] = uid
	}
	_, err := r.client.%s.Update().Where(%s.IDIn(uids...)).SetDeletedAt(now).Save(ctx)`, idVar, idVar, entityName, packageName)
		} else {
			return fmt.Sprintf(`// unsupported batch soft-delete for id type: %s`, idType)
		}
	case "exists":
		if idType == "string" {
			return fmt.Sprintf(`count, err := r.client.%s.Query().Where(%s.IDEQ(%s.String())).Count(ctx)`, entityName, packageName, idVar)
		} else if idType == "int64" {
			return fmt.Sprintf(`count, err := r.client.%s.Query().Where(%s.IDEQ(func() int64 {
		if i, err := %s.Int64(); err == nil {
			return i
		}
		return 0
	}())).Count(ctx)`, entityName, packageName, idVar)
		} else if isUUIDType(idType) {
			return fmt.Sprintf(`uid, parseErr := uuid.Parse(%s.String())
	if parseErr != nil {
		return false, fmt.Errorf("invalid uuid: %%w", parseErr)
	}
	count, err := r.client.%s.Query().Where(%s.IDEQ(uid)).Count(ctx)`, idVar, entityName, packageName)
		} else {
			return fmt.Sprintf(`count, err := r.client.%s.Query().Where(%s.IDEQ(%s)).Count(ctx)`, entityName, packageName, idVar)
		}
	case "batchDelete":
		if idType == "string" {
			return fmt.Sprintf(`stringIds := make([]string, len(ids))
	for i, id := range ids {
		stringIds[i] = id.String()
	}
	_, err := r.client.%s.Delete().Where(%s.IDIn(stringIds...)).Exec(ctx)`, entityName, packageName)
		} else if idType == "int64" {
			return fmt.Sprintf(`int64Ids := make([]int64, len(ids))
	for i, id := range ids {
		if intId, err := id.Int64(); err == nil {
			int64Ids[i] = intId
		}
	}
	_, err := r.client.%s.Delete().Where(%s.IDIn(int64Ids...)).Exec(ctx)`, entityName, packageName)
		} else if isUUIDType(idType) {
			return fmt.Sprintf(`uuidIds := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		uid, parseErr := uuid.Parse(id.String())
		if parseErr != nil {
			return fmt.Errorf("invalid uuid at index %%d: %%w", i, parseErr)
		}
		uuidIds[i] = uid
	}
	_, err := r.client.%s.Delete().Where(%s.IDIn(uuidIds...)).Exec(ctx)`, entityName, packageName)
		} else {
			return fmt.Sprintf(`_, err := r.client.%s.Delete().Where(%s.IDIn(ids...)).Exec(ctx)`, entityName, packageName)
		}
	default:
		return fmt.Sprintf("// Unknown operation: %s", operation)
	}
}

// setFieldCallReq generates a setter method call for a CreateRequest field (e.g., "SetName(req.Name)").
// For Nillable fields, uses SetNillable... to accept pointer types.
func setFieldCallReq(field *gen.Field, _ ...interface{}) string {
	if field.Nillable {
		return fmt.Sprintf("SetNillable%s(req.%s)", field.StructField(), field.StructField())
	}
	return fmt.Sprintf("Set%s(req.%s)", field.StructField(), field.StructField())
}

// fieldPredicate generates a type-assertion + Where predicate for a field.
// indent controls the indentation level of the generated code block.
// When skipEmpty is true, string checks include `&& v != ""`.
func fieldPredicate(field *gen.Field, node *gen.Type, indent string, skipEmpty bool) string {
	pkg := getEntityPackageName(node)
	name := field.StructField()
	ft := field.Type.String()

	where := func(cast, goType string) string {
		return fmt.Sprintf(`%sif v, ok := value.(%s); ok {
%s	query = query.Where(%s.%sEQ(v))
%s}`, indent, cast, indent, pkg, name, indent)
	}

	switch {
	case field.IsEnum():
		enumType := fmt.Sprintf("%s.%s", pkg, name)
		extra := ""
		if skipEmpty {
			extra = ` && v != ""`
		}
		// Try concrete enum type first (e.g., person.Gender), then fall back to string.
		// Go type assertions don't match underlying types, so both branches are needed.
		return fmt.Sprintf(`%sif v, ok := value.(%s); ok {
%s	query = query.Where(%s.%sEQ(v))
%s} else if v, ok := value.(string); ok%s {
%s	query = query.Where(%s.%sEQ(%s(v)))
%s}`, indent, enumType, indent, pkg, name, indent, extra, indent, pkg, name, enumType, indent)
	case ft == "string":
		extra := ""
		if skipEmpty {
			extra = ` && v != ""`
		}
		return fmt.Sprintf(`%sif v, ok := value.(string); ok%s {
%s	query = query.Where(%s.%sEQ(v))
%s}`, indent, extra, indent, pkg, name, indent)
	case ft == "int":
		return fmt.Sprintf(`%sif v, ok := value.(int); ok {
%s	query = query.Where(%s.%sEQ(v))
%s} else if v, ok := value.(int64); ok {
%s	query = query.Where(%s.%sEQ(int(v)))
%s}`, indent, indent, pkg, name, indent, indent, pkg, name, indent)
	case ft == "int32":
		return fmt.Sprintf(`%sif v, ok := value.(int32); ok {
%s	query = query.Where(%s.%sEQ(v))
%s} else if v, ok := value.(int64); ok {
%s	query = query.Where(%s.%sEQ(int32(v)))
%s}`, indent, indent, pkg, name, indent, indent, pkg, name, indent)
	case ft == "int64":
		return where("int64", ft)
	case ft == "bool":
		return where("bool", ft)
	case ft == "time.Time":
		return where("time.Time", ft)
	case ft == "float64":
		return where("float64", ft)
	case ft == "float32":
		return where("float32", ft)
	case isUUIDType(ft):
		return where("uuid.UUID", ft)
	case ft == "map[string]interface {}" || ft == "map[string]any":
		// JSON/map fields cannot be used as equality filters; skip silently.
		return fmt.Sprintf("%s// skip: map field %s is not filterable", indent, field.Name)
	default:
		return fmt.Sprintf("%s// unsupported field type: %s", indent, ft)
	}
}

// searchMethod generates a filter predicate for Search/Count methods (nested indentation, skips empty strings).
func searchMethod(field *gen.Field, node *gen.Type) string {
	return fieldPredicate(field, node, "\t\t\t", true)
}

// findByMethod generates a filter predicate for FindBy methods (standard indentation).
func findByMethod(field *gen.Field, node *gen.Type) string {
	return fieldPredicate(field, node, "\t\t", false)
}

// last checks if this is the last element in slice
func last(slice []*gen.Field) *gen.Field {
	if len(slice) == 0 {
		return nil
	}
	return slice[len(slice)-1]
}
