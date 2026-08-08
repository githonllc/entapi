package entapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/githonllc/entapi/api"
)

// exemptStatuses records handler arms that are intentionally absent from the
// OpenAPI error table. The list, get, and delete branches carry a syntactically
// present IsAlreadyExists -> StatusConflict arm that is unreachable in practice:
// a unique-constraint violation only arises from a write, and a foreign-key
// violation fails closed to 500. This was established in #76's review; this
// guard was added in #89. Do not remove the arms from the template: doing so
// would rewrite every committed fixture and is outside this guard's scope.
var exemptStatuses = map[api.Op][]int{
	api.OpList:   {409},
	api.OpGet:    {409},
	api.OpDelete: {409},
}

// TestErrorStatusesByOpMatchesHandlerTemplate prevents the hand-maintained
// OpenAPI error table from drifting from the generated handler branches. This
// belongs in a test rather than generation because a template function that
// parses another template is worse than an explicit drift guard.
func TestErrorStatusesByOpMatchesHandlerTemplate(t *testing.T) {
	content, err := templateFS.ReadFile("templates/handler.tmpl")
	if err != nil {
		t.Fatalf("handler.tmpl operation sections have no status code because the template cannot be read: %v", err)
	}
	template := string(content)

	const markerPrefix = `eq $name "`
	if got, want := strings.Count(template, markerPrefix), len(errorStatusesByOp); got != want {
		t.Fatalf("handler.tmpl operation set has no single status code: found %d branch markers, but errorStatusesByOp has %d operations", got, want)
	}

	type section struct {
		op    api.Op
		start int
	}
	sections := make([]section, 0, len(errorStatusesByOp))
	for op := range errorStatusesByOp {
		marker := fmt.Sprintf("eq $name %q", string(op))
		if count := strings.Count(template, marker); count != 1 {
			t.Fatalf("handler.tmpl %s operation has no unambiguous status code section: marker %q occurs %d times, want exactly once", op, marker, count)
		}
		sections = append(sections, section{op: op, start: strings.Index(template, marker)})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].start < sections[j].start })

	statusPattern := regexp.MustCompile(`http\.Status[A-Z][A-Za-z]*`)
	statusCodeByIdentifier := map[string]int{
		"http.StatusOK":                    200,
		"http.StatusCreated":               201,
		"http.StatusNoContent":             204,
		"http.StatusBadRequest":            400,
		"http.StatusNotFound":              404,
		"http.StatusConflict":              409,
		"http.StatusRequestEntityTooLarge": 413,
		"http.StatusUnsupportedMediaType":  415,
		"http.StatusUnprocessableEntity":   422,
		"http.StatusInternalServerError":   500,
	}

	for _, name := range statusPattern.FindAllString(template[:sections[0].start], -1) {
		if name == "http.StatusText" {
			continue
		}
		code, ok := statusCodeByIdentifier[name]
		if !ok {
			t.Fatalf("handler.tmpl preamble applies unmapped status %s to every operation; extend statusCodeByIdentifier", name)
		}
		t.Errorf("handler.tmpl preamble writes status %d outside every operation branch", code)
	}

	scanStatuses := func(op api.Op, body string) map[int]bool {
		statuses := map[int]bool{}
		for _, name := range statusPattern.FindAllString(body, -1) {
			if name == "http.StatusText" {
				continue
			}
			code, ok := statusCodeByIdentifier[name]
			if !ok {
				t.Fatalf("handler.tmpl %s operation uses unmapped status %s; extend statusCodeByIdentifier", op, name)
			}
			if code >= 400 {
				statuses[code] = true
			}
		}
		return statuses
	}

	for i, current := range sections {
		end := len(template)
		if i+1 < len(sections) {
			end = sections[i+1].start
		}
		scanned := scanStatuses(current.op, template[current.start:end])
		table := make(map[int]bool, len(errorStatusesByOp[current.op]))
		for _, code := range errorStatusesByOp[current.op] {
			table[code] = true
		}
		exempt := make(map[int]bool, len(exemptStatuses[current.op]))
		for _, code := range exemptStatuses[current.op] {
			exempt[code] = true
			if table[code] {
				t.Errorf("errorStatusesByOp and exemptStatuses both list %d for %s; remove the stale exemption", code, current.op)
			}
		}

		for code := range scanned {
			if !table[code] && !exempt[code] {
				t.Errorf("handler.tmpl %s branch writes %d, but errorStatusesByOp omits it", current.op, code)
			}
		}
		for code := range table {
			if !scanned[code] {
				t.Errorf("errorStatusesByOp lists %d for %s, but handler.tmpl's branch never writes it", code, current.op)
			}
		}
		for code := range exempt {
			if !scanned[code] {
				t.Errorf("handler.tmpl %s branch never writes exempt status %d; remove the stale exemption", current.op, code)
			}
		}
	}
}
