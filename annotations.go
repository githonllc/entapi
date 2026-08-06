package entdomain

// FieldScope defines the usage scope of a field at the handler layer.
// Key principles:
// 1. These scopes only affect handler-layer HTTP request/response struct generation
// 2. The service layer operates directly on ent entities, unrestricted by these scopes
// 3. Layered architecture: Handler (HTTP) -> Service (business logic) -> ent.Client (data access)
type FieldScope string

const (
	// ScopeCreate indicates the field can be populated from an HTTP create request.
	// Scope of influence:
	// - Handler layer: the field appears in the CreateRequest struct
	// - Service layer: unrestricted, can create and set this field
	// - Repository layer: unrestricted, can create and set this field
	ScopeCreate FieldScope = "create"

	// ScopeUpdate indicates the field can be populated from an HTTP update request.
	// Scope of influence:
	// - Handler layer: the field appears in the UpdateRequest struct
	// - Service layer: unrestricted, can update this field
	// - Repository layer: unrestricted, can update this field
	ScopeUpdate FieldScope = "update"

	// ScopeQuery indicates the field supports searching and filtering via the HTTP API.
	// Scope of influence:
	// - Handler layer: the field appears in the QueryParams struct
	// - Service layer: can be used to build query conditions
	// - Repository layer: can be used to build database queries
	ScopeQuery FieldScope = "query"

	// ScopeResponse indicates the field appears in the HTTP response.
	// Scope of influence:
	// - Handler layer: the field appears in the Response struct
	// - Service layer: accessible via ent entity fields
	ScopeResponse FieldScope = "response"
)

// AllFieldScopes contains every defined FieldScope value. Use this to
// create fields that are accessible in all handler-layer operations.
var AllFieldScopes = []FieldScope{ScopeCreate, ScopeUpdate, ScopeQuery, ScopeResponse}

// FieldMetadata holds field metadata for future documentation and API spec generation.
// RESERVED: These fields are stored in annotations but not yet consumed by code generation
// templates. They will be used when OpenAPI/Swagger spec generation is implemented.
type FieldMetadata struct {
	// Title is the user-friendly field name
	Title string `json:"title,omitempty"`

	// Format is the field format (e.g., email, date-time, uuid, etc.)
	Format string `json:"format,omitempty"`

	// Pattern is the regular expression pattern
	Pattern string `json:"pattern,omitempty"`

	// Minimum is the minimum value (for numeric types)
	Minimum *float64 `json:"minimum,omitempty"`

	// Maximum is the maximum value (for numeric types)
	Maximum *float64 `json:"maximum,omitempty"`

	// MinLength is the minimum length (for string types)
	MinLength *int `json:"minLength,omitempty"`

	// MaxLength is the maximum length (for string types)
	MaxLength *int `json:"maxLength,omitempty"`

	// Enum holds the enumeration values
	Enum []interface{} `json:"enum,omitempty"`

	// ReadOnly indicates whether the field is read-only
	ReadOnly bool `json:"readOnly,omitempty"`

	// WriteOnly indicates whether the field is write-only
	WriteOnly bool `json:"writeOnly,omitempty"`

	// Deprecated indicates whether the field is deprecated
	Deprecated bool `json:"deprecated,omitempty"`

	// Tags holds tags used for grouping
	Tags []string `json:"tags,omitempty"`
}

// DomainField is the domain field annotation.
// Core design principles:
// 1. Scopes only control handler-layer HTTP behavior
// 2. The service and repository layers can always access and operate on all fields
// 3. This design ensures the business logic layer is not restricted by HTTP API limitations
type DomainField struct {
	// Scopes defines the field's usage scope at the handler layer.
	// Only affects HTTP request/response struct generation; does not affect the service/repository layers.
	Scopes []FieldScope `json:"scopes,omitempty"`

	// Required specifies whether the field is required within the given scopes at the handler layer.
	// Only affects HTTP request validation; does not affect service/repository layer business logic.
	Required map[FieldScope]bool `json:"required,omitempty"`

	// Validation holds validation rules (primarily for handler-layer HTTP request validation)
	Validation map[string]interface{} `json:"validation,omitempty"`

	// Description is the field description
	Description string `json:"description,omitempty"`

	// Example is the example value
	Example interface{} `json:"example,omitempty"`

	// Searchable indicates whether the field is searchable (affects QueryParams and query method generation)
	Searchable bool `json:"searchable,omitempty"`

	// Sortable indicates whether the field is sortable (affects sorting-related API and query method generation)
	Sortable bool `json:"sortable,omitempty"`

	// Filterable marks the field as filterable in query APIs
	Filterable bool `json:"filterable,omitempty"`

	// Metadata contains additional field metadata for documentation and API spec generation
	Metadata *FieldMetadata `json:"metadata,omitempty"`
}

// Name implements the schema.Annotation interface
func (DomainField) Name() string {
	return "DomainField"
}

// DomainConfig is the entity-level configuration annotation.
//
// It carries no options. Its only field, EntityName, was removed on #17: it
// had zero readers outside its own tests, and no successor — generation names
// every emitted type from gen.Type.Name, and an override would have had to be
// threaded through three templates to mean anything.
//
// Nothing reads this annotation today. Attaching it to a schema changes no
// generated output. It is kept as a type only because whether to publish an
// entity-level annotation at all is a surface decision on #17, not a
// consequence of removing the field; do not add options to it without one.
type DomainConfig struct{}

// Name implements the schema.Annotation interface.
func (DomainConfig) Name() string {
	return "DomainConfig"
}

// Core annotation builder functions

// NewDomainField creates an empty domain field annotation
func NewDomainField() DomainField {
	return DomainField{}
}

// DomainFieldWithScopes creates a field annotation with the specified scopes.
// This is the most basic builder for custom scope combinations.
func DomainFieldWithScopes(scopes ...FieldScope) DomainField {
	return DomainField{
		Scopes: copyScopes(scopes),
	}
}

// copyScopes, copyEnum and copyTags return a copy of the slice, so a built
// annotation never shares a backing array with its caller's slice, with a
// package-level var such as AllFieldScopes, or with another chain forked from
// the same base annotation. nil stays nil, so an unset field keeps encoding as
// absent rather than as an empty list.
func copyScopes(scopes []FieldScope) []FieldScope {
	if scopes == nil {
		return nil
	}
	out := make([]FieldScope, len(scopes))
	copy(out, scopes)
	return out
}

func copyEnum(values []interface{}) []interface{} {
	if values == nil {
		return nil
	}
	out := make([]interface{}, len(values))
	copy(out, values)
	return out
}

func copyTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

// copyValidation returns a copy of the validation rules, so a caller that
// keeps a reference to the map it passed in cannot mutate the annotation it
// already built. nil stays nil, like the slice helpers above.
//
// The copy is shallow and deliberately stops at the map: the interface{}
// values are handed straight over, so a caller that passes a nested map or
// slice as a rule value still shares that value. Deep-copying arbitrary rule
// values is out of scope here.
func copyValidation(rules map[string]interface{}) map[string]interface{} {
	if rules == nil {
		return nil
	}
	out := make(map[string]interface{}, len(rules))
	for k, v := range rules {
		out[k] = v
	}
	return out
}

// DefaultField creates a standard business field (fully accessible at the HTTP layer).
// Suitable for: most business fields such as name, email, address, etc.
// Layer impact:
// - Handler layer: can be populated from HTTP requests, supports querying, appears in responses
// - Service layer: fully accessible, unrestricted by scopes
// - Repository layer: fully accessible, unrestricted by scopes
func DefaultField() DomainField {
	return DomainField{
		// A copy: handing out the package-level slice would let any caller
		// that assigns to an element corrupt AllFieldScopes for every
		// subsequent caller.
		Scopes: copyScopes(AllFieldScopes),
	}.AsSearchable().AsFilterable().AsSortable()
}

// InputOnlyField creates an HTTP-input-only field (excluded from HTTP responses).
// Suitable for: passwords, secrets and anything else that must never be
// serialised back to a client. Withholding ScopeResponse is what keeps the
// field out of the response struct, and it is the only mechanism that does:
// there is no separate marker to set, and none to forget.
// Layer impact:
// - Handler layer: can be populated from HTTP create/update requests, but excluded from responses
// - Service layer: fully accessible; can internally set, read, and query this field
// - Repository layer: fully accessible; can create, update, read, and query this field
func InputOnlyField() DomainField {
	return DomainField{
		Scopes: []FieldScope{ScopeCreate, ScopeUpdate},
	}
}

// OutputOnlyField creates a system-managed field (read-only at the HTTP layer).
// Suitable for: ID, created_at, updated_at, and other system fields.
// Layer impact:
// - Handler layer: cannot be set via HTTP requests, only appears in responses and supports querying
// - Service layer: fully accessible; can internally create and set this field
// - Repository layer: fully accessible; can create, set, and query this field
// Important: although the HTTP layer cannot modify this field, the service/repository layers can still operate on it internally.
func OutputOnlyField() DomainField {
	return DomainField{
		Scopes: []FieldScope{ScopeQuery, ScopeResponse},
	}.AsSearchable().AsFilterable().AsSortable()
}

// CreateOnlyField creates a write-once field (immutable after creation at the HTTP layer).
// Suitable for: creator ID, initial status, foreign keys, etc.
// Layer impact:
// - Handler layer: can only be set during HTTP creation, appears in responses, supports querying
// - Service layer: fully accessible; business logic can modify it at any time
// - Repository layer: fully accessible; can create, update, and query at any time
func CreateOnlyField() DomainField {
	return DomainField{
		Scopes: []FieldScope{ScopeCreate, ScopeQuery, ScopeResponse},
	}.AsSearchable().AsFilterable().AsSortable()
}

// IdField creates an entity identifier field.
// Suitable for: primary key ID.
// Layer impact:
// - Handler layer: cannot be set via HTTP requests, only appears in responses, supports querying
// - Service layer: fully accessible; generated and set internally by the system
// - Repository layer: fully accessible; auto-generated or set when creating entities
func IdField() DomainField {
	return OutputOnlyField().
		WithDescription("Unique entity identifier").
		AsReadOnly()
}

// AuditLogField creates an audit field.
// Suitable for: audit log related fields.
// Layer impact:
// - Handler layer: cannot be set via HTTP requests, only appears in responses, supports querying
// - Service layer: fully accessible; the system internally records audit information
// - Repository layer: fully accessible; can create and update audit records
func AuditLogField() DomainField {
	return OutputOnlyField().
		AsReadOnly()
}

// Fluent builder methods

// WithRequired marks the field as required within the specified scope.
//
// The receiver is taken by value, but that copy is shallow: writing into the
// receiver's map would be visible to every other chain forked from the same
// base annotation. So always allocate a fresh map and copy the existing
// entries into it, even when the receiver's map is non-nil.
func (d DomainField) WithRequired(scope FieldScope) DomainField {
	required := make(map[FieldScope]bool, len(d.Required)+1)
	for k, v := range d.Required {
		required[k] = v
	}
	required[scope] = true
	d.Required = required
	return d
}

// WithValidation adds validation rules to the field
func (d DomainField) WithValidation(rules map[string]interface{}) DomainField {
	d.Validation = copyValidation(rules)
	return d
}

// WithDescription sets the field description
func (d DomainField) WithDescription(desc string) DomainField {
	d.Description = desc
	return d
}

// WithExample sets an example value for the field
func (d DomainField) WithExample(example interface{}) DomainField {
	d.Example = example
	return d
}

// AsSearchable marks the field as searchable
func (d DomainField) AsSearchable() DomainField {
	d.Searchable = true
	return d
}

// AsSortable marks the field as sortable
func (d DomainField) AsSortable() DomainField {
	d.Sortable = true
	return d
}

// AsFilterable marks the field as filterable
func (d DomainField) AsFilterable() DomainField {
	d.Filterable = true
	return d
}

// Metadata related methods

// ensureMetadata gives the returned DomainField a *FieldMetadata that nobody
// else holds, so the caller can write through it freely.
//
// It always allocates. The receiver is copied by value but the Metadata
// pointer is not, so reusing an existing pointee would make every chain forked
// from the same base annotation write into one shared FieldMetadata. Enum and
// Tags are slices and get their own backing arrays for the same reason.
func (d DomainField) ensureMetadata() DomainField {
	if d.Metadata == nil {
		d.Metadata = &FieldMetadata{}
		return d
	}
	metadata := *d.Metadata
	metadata.Enum = copyEnum(metadata.Enum)
	metadata.Tags = copyTags(metadata.Tags)
	d.Metadata = &metadata
	return d
}

// WithMetadata sets field metadata directly.
func (d DomainField) WithMetadata(metadata FieldMetadata) DomainField {
	d.Metadata = &metadata
	return d
}

// WithTitle sets the field title for documentation and API specs.
func (d DomainField) WithTitle(title string) DomainField {
	d = d.ensureMetadata()
	d.Metadata.Title = title
	return d
}

// WithFormat sets the field format (e.g., "email", "date-time", "uuid").
func (d DomainField) WithFormat(format string) DomainField {
	d = d.ensureMetadata()
	d.Metadata.Format = format
	return d
}

// WithPattern sets the regular expression pattern for string validation.
func (d DomainField) WithPattern(pattern string) DomainField {
	d = d.ensureMetadata()
	d.Metadata.Pattern = pattern
	return d
}

// WithRange sets the minimum and maximum numeric value constraints.
func (d DomainField) WithRange(min, max *float64) DomainField {
	d = d.ensureMetadata()
	d.Metadata.Minimum = min
	d.Metadata.Maximum = max
	return d
}

// WithLength sets the minimum and maximum string length constraints.
func (d DomainField) WithLength(min, max *int) DomainField {
	d = d.ensureMetadata()
	d.Metadata.MinLength = min
	d.Metadata.MaxLength = max
	return d
}

// WithEnum sets the allowed enumeration values for the field.
func (d DomainField) WithEnum(values ...interface{}) DomainField {
	d = d.ensureMetadata()
	d.Metadata.Enum = copyEnum(values)
	return d
}

// AsReadOnly marks the field as read-only in API specifications.
func (d DomainField) AsReadOnly() DomainField {
	d = d.ensureMetadata()
	d.Metadata.ReadOnly = true
	return d
}

// AsWriteOnly marks the field as write-only in API specifications.
func (d DomainField) AsWriteOnly() DomainField {
	d = d.ensureMetadata()
	d.Metadata.WriteOnly = true
	return d
}

// AsDeprecated marks the field as deprecated.
func (d DomainField) AsDeprecated() DomainField {
	d = d.ensureMetadata()
	d.Metadata.Deprecated = true
	return d
}

// WithTags adds grouping tags to the field for documentation organization.
func (d DomainField) WithTags(tags ...string) DomainField {
	d = d.ensureMetadata()
	d.Metadata.Tags = copyTags(tags)
	return d
}
