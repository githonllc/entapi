package entapi

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/githonllc/entapi/api"
	entapiruntime "github.com/githonllc/entapi/runtime"
)

// exemptStatuses records handler arms that are intentionally absent from the
// OpenAPI error table. The list, get, and delete branches carry a syntactically
// present IsAlreadyExists -> StatusConflict arm that is unreachable in practice:
// a unique-constraint violation only arises from a write, and a foreign-key
// violation fails closed to 500. This was established in #76's review; this
// guard was added in #89. Do not remove the arms from the template: doing so
// would rewrite every committed fixture and is outside this guard's scope.
//
// List's 409 no longer comes from a literal IsAlreadyExists arm — #103 replaced
// that switch with entapi.Status, which classifies the same sentinel — so the
// exemption is reached through the probe below instead of through the text
// scan. The values are unchanged, and so is the reason they are exempt.
var exemptStatuses = map[api.Op][]int{
	api.OpList:   {409},
	api.OpGet:    {409},
	api.OpDelete: {409},
}

// TestErrorStatusesByOpMatchesHandlerTemplate prevents the hand-maintained
// OpenAPI error table from drifting from the generated handler branches. This
// belongs in a test rather than generation because a template function that
// parses another template is worse than an explicit drift guard.
//
// Since #103 the handler bodies no longer spell every status they can write:
// entapi.BindJSON and entapi.Status decide most of them at run time. So each
// branch's expected set is the union of two halves -- the literal http.Status
// identifiers still in the text, plus the statuses obtained by CALLING
// runtime.Status over a list of sentinels at each call site's onValidation
// argument. Importing the runtime from this test is the same direction
// funcs_openapi.go already takes for a non-test import.
//
// The residue, stated rather than hidden, is now in two places. The text half
// sees an error status only where handler.tmpl spells it as an http.Status<Name>
// identifier: a numeric literal, or a status reached through a variable or a
// helper constant, is invisible. The probe half is only as complete as the two
// sentinel lists below, which are HAND-CHOSEN -- a sentinel added to
// runtime.Status later maps to a status this guard never sees until someone
// adds it to middleSentinels or bindSentinels here. Both asymmetries weaken only
// the ADDITION direction -- a new status the table does not list could slip
// past. Removals stay loud, because they are caught from the table's side, which
// this test reads as Go values rather than as text.
func TestErrorStatusesByOpMatchesHandlerTemplate(t *testing.T) {
	content, err := templateFS.ReadFile("templates/handler.tmpl")
	if err != nil {
		t.Fatalf("reading templates/handler.tmpl from templateFS: %v", err)
	}
	template := string(content)

	const markerPrefix = `eq $name "`
	if got, want := strings.Count(template, markerPrefix), len(errorStatusesByOp); got != want {
		t.Fatalf("handler.tmpl has %d operation branch markers (eq $name \"...\"), but errorStatusesByOp has %d operations; update whichever side changed", got, want)
	}

	type section struct {
		op    api.Op
		start int
	}
	sections := make([]section, 0, len(errorStatusesByOp))
	for op := range errorStatusesByOp {
		marker := fmt.Sprintf("eq $name %q", string(op))
		if count := strings.Count(template, marker); count != 1 {
			t.Fatalf("handler.tmpl %s branch cannot be delimited: marker %q occurs %d times, want exactly once", op, marker, count)
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

	// The sentinels each runtime entry point classifies. Hand-chosen; see the
	// residue paragraph above.
	opaque := errors.New("an error carrying no entapi sentinel")
	middleSentinels := []error{
		entapiruntime.ErrNotFound, entapiruntime.ErrAlreadyExists,
		entapiruntime.ErrValidation, opaque,
	}
	bindSentinels := []error{
		entapiruntime.ErrUnsupportedMediaType, entapiruntime.ErrRequestTooLarge,
		entapiruntime.ErrValidation,
	}
	statusCallPattern := regexp.MustCompile(`entapi\.Status\([A-Za-z]+, (http\.Status[A-Za-z]+)\)`)

	scanStatuses := func(op api.Op, body string) map[int]bool {
		statuses := map[int]bool{}
		probe := func(onValidation int, sentinels []error) {
			for _, err := range sentinels {
				if code := entapiruntime.Status(err, onValidation); code >= 400 {
					statuses[code] = true
				}
			}
		}
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

		// Every entapi.Status call site classifies the middle step's error.
		calls := statusCallPattern.FindAllStringSubmatchIndex(body, -1)
		onValidationAfter := func(offset int) (int, bool) {
			for _, call := range calls {
				if call[0] < offset {
					continue
				}
				return statusCodeByIdentifier[body[call[2]:call[3]]], true
			}
			return 0, false
		}
		for _, call := range calls {
			ident := body[call[2]:call[3]]
			code, ok := statusCodeByIdentifier[ident]
			if !ok {
				t.Fatalf("handler.tmpl %s operation passes unmapped status %s to entapi.Status; extend statusCodeByIdentifier", op, ident)
			}
			probe(code, middleSentinels)
		}

		// Every entapi.BindJSON call site classifies the bind step's error, and
		// reports it through the entapi.Status call that immediately follows.
		for offset := 0; ; {
			idx := strings.Index(body[offset:], "entapi.BindJSON")
			if idx < 0 {
				break
			}
			offset += idx + len("entapi.BindJSON")
			onValidation, ok := onValidationAfter(offset)
			if !ok {
				t.Fatalf("handler.tmpl %s operation calls entapi.BindJSON with no entapi.Status call after it to read onValidation from", op)
			}
			probe(onValidation, bindSentinels)
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
