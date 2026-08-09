package entapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

type classifiedError struct {
	class error
	cause error
}

func (e classifiedError) Error() string { return e.cause.Error() }

func (e classifiedError) Unwrap() []error { return []error{e.class, e.cause} }

func classifyError(class, cause error) error {
	return classifiedError{class: class, cause: cause}
}

// BindJSON reads a JSON request into dst after enforcing the generated HTTP
// handlers' media-type, size, and unknown-field rules. The 1 MiB body limit is
// intentionally hardcoded: the bind hardenings have no knobs. w is used only
// by http.MaxBytesReader; BindJSON never writes a response.
func BindJSON(w http.ResponseWriter, r *http.Request, tags []string, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return classifyError(ErrUnsupportedMediaType, errors.New("content type must be application/json"))
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return classifyError(ErrRequestTooLarge, err)
		}
		return classifyError(ErrValidation, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return classifyError(ErrValidation, err)
	}
	unknownField := ""
	for key := range raw {
		known := false
		for _, tag := range tags {
			if key == tag {
				known = true
				break
			}
		}
		if !known && (unknownField == "" || key < unknownField) {
			unknownField = key
		}
	}
	if unknownField != "" {
		return &FieldError{
			Field: unknownField,
			Err:   fmt.Errorf("%w: unknown field %q", ErrValidation, unknownField),
		}
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return classifyError(ErrValidation, err)
	}
	return nil
}

// Status returns the HTTP status for err. Validation errors use onValidation
// because binding failures are 400 while create and patch validation failures
// are 422.
//
// The two bind sentinels are classified wherever they appear, so a custom With
// replacement that returns ErrUnsupportedMediaType or ErrRequestTooLarge now
// answers 415 or 413 rather than 500. Generated wiring cannot reach that — it
// never returns either sentinel — which is why the OpenAPI document does not
// grow those statuses for operations whose bodies it describes.
func Status(err error, onValidation int) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ErrUnsupportedMediaType):
		return http.StatusUnsupportedMediaType
	case errors.Is(err, ErrRequestTooLarge):
		return http.StatusRequestEntityTooLarge
	case IsNotFound(err):
		return http.StatusNotFound
	case IsAlreadyExists(err):
		return http.StatusConflict
	case IsValidation(err):
		return onValidation
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON writes v as a bare JSON response followed by one newline. It
// marshals before touching the response so a marshal failure can still be
// written as a clean 500 problem response.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(append(body, '\n'))
	return err
}
