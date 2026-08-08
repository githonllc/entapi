// Package api contains the schema-time annotations understood by entapi.
//
// The package deliberately depends only on Ent's schema annotation interface
// and the standard library. Generated application code never imports it.
package api

import "entgo.io/ent/schema"

// Op names an HTTP operation that Resource can exclude.
//
// It is string-backed because Ent annotations cross a JSON serialization
// boundary. String values retain their meaning there; numeric enum values do
// not retain their Go type.
type Op string

const (
	// OpCreate names the create operation.
	OpCreate Op = "create"
	// OpPatch names the partial-update operation.
	OpPatch Op = "patch"
	// OpDelete names the delete operation.
	OpDelete Op = "delete"
	// OpGet names the single-resource read operation.
	OpGet Op = "get"
	// OpList names the resource-list operation.
	OpList Op = "list"
)

// ResourceAnnotation opts an Ent type into entapi generation.
type ResourceAnnotation struct {
	// ExceptOps records the public operations removed from the resource.
	ExceptOps []Op `json:"except,omitempty"`
}

// Resource opts an Ent type into entapi generation.
func Resource() ResourceAnnotation { return ResourceAnnotation{} }

// Name implements schema.Annotation.
func (ResourceAnnotation) Name() string { return "EntAPIResource" }

// Except excludes operations from the resource's HTTP surface.
//
// The service-layer wiring remains available unless the create surface is
// provably unusable. Repeating an operation has no additional effect.
func (a ResourceAnnotation) Except(ops ...Op) ResourceAnnotation {
	a.ExceptOps = append([]Op(nil), a.ExceptOps...)
	for _, op := range ops {
		if !containsOp(a.ExceptOps, op) {
			a.ExceptOps = append(a.ExceptOps, op)
		}
	}
	return a
}

// Merge implements schema.Merger by taking the union of excluded operations.
func (a ResourceAnnotation) Merge(other schema.Annotation) schema.Annotation {
	switch v := other.(type) {
	case ResourceAnnotation:
		return a.Except(v.ExceptOps...)
	case *ResourceAnnotation:
		if v != nil {
			return a.Except(v.ExceptOps...)
		}
	}
	return a
}

func containsOp(ops []Op, want Op) bool {
	for _, op := range ops {
		if op == want {
			return true
		}
	}
	return false
}

// FieldAnnotation records deviations from the HTTP shape Ent already defines.
type FieldAnnotation struct {
	// Hidden removes the field from every HTTP surface.
	Hidden bool `json:"hidden,omitempty"`
	// ReadOnly removes the field from create and patch requests.
	ReadOnly bool `json:"read_only,omitempty"`
	// Searchable adds the field to free-text search.
	Searchable bool `json:"searchable,omitempty"`
	// Filterable adds the field's Ent predicates to structured filtering.
	Filterable bool `json:"filterable,omitempty"`
	// Sortable adds the field to the sort allow-list.
	Sortable bool `json:"sortable,omitempty"`
}

// Hidden keeps a field out of every generated HTTP surface.
func Hidden() FieldAnnotation { return FieldAnnotation{Hidden: true} }

// ReadOnly keeps a field out of create and patch requests while retaining it
// in responses.
func ReadOnly() FieldAnnotation { return FieldAnnotation{ReadOnly: true} }

// Searchable adds a field to free-text search.
func Searchable() FieldAnnotation { return FieldAnnotation{Searchable: true} }

// Filterable adds a field's supported predicates to structured filtering.
func Filterable() FieldAnnotation { return FieldAnnotation{Filterable: true} }

// Sortable adds a field to the generated sort allow-list.
func Sortable() FieldAnnotation { return FieldAnnotation{Sortable: true} }

// Name implements schema.Annotation.
func (FieldAnnotation) Name() string { return "EntAPIField" }

// Merge implements schema.Merger by taking the union of deviation words.
func (a FieldAnnotation) Merge(other schema.Annotation) schema.Annotation {
	var b FieldAnnotation
	switch v := other.(type) {
	case FieldAnnotation:
		b = v
	case *FieldAnnotation:
		if v == nil {
			return a
		}
		b = *v
	default:
		return a
	}
	a.Hidden = a.Hidden || b.Hidden
	a.ReadOnly = a.ReadOnly || b.ReadOnly
	a.Searchable = a.Searchable || b.Searchable
	a.Filterable = a.Filterable || b.Filterable
	a.Sortable = a.Sortable || b.Sortable
	return a
}

// EdgeAnnotation records response expansion for an Ent edge.
type EdgeAnnotation struct {
	// Expand includes the edge as the target resource's summary.
	Expand bool `json:"expand,omitempty"`
	// Key overrides the expanded edge's JSON key when non-empty.
	Key string `json:"json_key,omitempty"`
}

// Expand includes an edge in responses as the target resource's summary.
func Expand() EdgeAnnotation { return EdgeAnnotation{Expand: true} }

// Name implements schema.Annotation.
func (EdgeAnnotation) Name() string { return "EntAPIEdge" }

// JSONKey overrides the expanded edge's JSON key. An empty key restores the
// default edge storage key when used as a standalone annotation.
func (a EdgeAnnotation) JSONKey(key string) EdgeAnnotation {
	a.Key = key
	return a
}

// Merge implements schema.Merger. Expand is unioned and the later non-empty
// JSON key wins.
func (a EdgeAnnotation) Merge(other schema.Annotation) schema.Annotation {
	var b EdgeAnnotation
	switch v := other.(type) {
	case EdgeAnnotation:
		b = v
	case *EdgeAnnotation:
		if v == nil {
			return a
		}
		b = *v
	default:
		return a
	}
	a.Expand = a.Expand || b.Expand
	if b.Key != "" {
		a.Key = b.Key
	}
	return a
}
