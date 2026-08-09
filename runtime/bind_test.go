package entapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type bindPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type failingReadCloser struct {
	err error
}

func (r failingReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (failingReadCloser) Close() error               { return nil }

func TestBindJSONClassifiesEveryFailureWithoutWriting(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name        string
		contentType string
		body        string
		bodyErr     error
		tags        []string
		dst         func() any
		wantClass   error
		wantStatus  int
		wantError   string
	}{
		{
			name:        "malformed content type",
			contentType: "application/json; charset",
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrUnsupportedMediaType,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantError:   "content type must be application/json",
		},
		{
			name:        "non-JSON content type",
			contentType: "text/plain",
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrUnsupportedMediaType,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantError:   "content type must be application/json",
		},
		{
			name:        "body read",
			contentType: "application/json",
			bodyErr:     readErr,
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrValidation,
			wantStatus:  http.StatusBadRequest,
			wantError:   "read failed",
		},
		{
			name:        "request too large",
			contentType: "application/json",
			body:        strings.Repeat("x", (1<<20)+1),
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrRequestTooLarge,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantError:   "http: request body too large",
		},
		{
			name:        "raw JSON decode",
			contentType: "application/json",
			body:        "{",
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrValidation,
			wantStatus:  http.StatusBadRequest,
			wantError:   "unexpected end of JSON input",
		},
		{
			name:        "unknown field",
			contentType: "application/json",
			body:        `{"zeta":1,"alpha":2,"name":"article"}`,
			tags:        []string{"name"},
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrValidation,
			wantStatus:  http.StatusBadRequest,
			wantError:   `alpha: validation failed: unknown field "alpha"`,
		},
		{
			name:        "destination JSON decode",
			contentType: "application/json",
			body:        `{"count":"many"}`,
			tags:        []string{"count"},
			dst:         func() any { return &bindPayload{} },
			wantClass:   ErrValidation,
			wantStatus:  http.StatusBadRequest,
			wantError:   "json: cannot unmarshal string into Go struct field bindPayload.count of type int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.contentType)
			if tt.bodyErr != nil {
				request.Body = failingReadCloser{err: tt.bodyErr}
			}

			err := BindJSON(recorder, request, tt.tags, tt.dst())
			if err == nil {
				t.Fatal("BindJSON() error = nil")
			}
			if !errors.Is(err, tt.wantClass) {
				t.Errorf("BindJSON() error = %v, want class %v", err, tt.wantClass)
			}
			matches := 0
			for _, class := range []error{ErrUnsupportedMediaType, ErrRequestTooLarge, ErrValidation} {
				if errors.Is(err, class) {
					matches++
				}
			}
			if matches != 1 {
				t.Errorf("BindJSON() error matched %d classes, want exactly 1", matches)
			}
			if got := Status(err, http.StatusBadRequest); got != tt.wantStatus {
				t.Errorf("Status(BindJSON error, 400) = %d, want %d", got, tt.wantStatus)
			}
			if err.Error() != tt.wantError {
				t.Errorf("BindJSON() error = %q, want %q", err, tt.wantError)
			}
			if recorder.Code != http.StatusOK {
				t.Errorf("response status = %d, want 200", recorder.Code)
			}
			if recorder.Body.Len() != 0 {
				t.Errorf("response body = %q, want empty", recorder.Body.String())
			}
		})
	}
}

func TestBindJSONUnknownFieldIsLexicallySmallestFieldError(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"zeta":1,"name":"article","alpha":2}`,
	))
	request.Header.Set("Content-Type", "application/json")

	err := BindJSON(recorder, request, []string{"name"}, &bindPayload{})
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("BindJSON() error type = %T, want *FieldError", err)
	}
	if fieldErr.Field != "alpha" {
		t.Errorf("FieldError.Field = %q, want alpha", fieldErr.Field)
	}
	if !errors.Is(fieldErr.Err, ErrValidation) {
		t.Errorf("FieldError.Err = %v, want ErrValidation", fieldErr.Err)
	}
}

func TestBindJSONFillsDestination(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"name":"article","count":3}`,
	))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	var got bindPayload

	if err := BindJSON(recorder, request, []string{"name", "count"}, &got); err != nil {
		t.Fatalf("BindJSON() error = %v", err)
	}
	if got != (bindPayload{Name: "article", Count: 3}) {
		t.Errorf("BindJSON() destination = %+v, want {Name:article Count:3}", got)
	}
}

func TestStatusClassifiesErrorsInPriorityOrder(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", want: 0},
		{
			name: "unsupported media type",
			err: errors.Join(
				ErrUnsupportedMediaType,
				ErrRequestTooLarge,
				ErrNotFound,
				ErrAlreadyExists,
				ErrValidation,
			),
			want: http.StatusUnsupportedMediaType,
		},
		{
			name: "request too large",
			err:  errors.Join(ErrRequestTooLarge, ErrNotFound, ErrAlreadyExists, ErrValidation),
			want: http.StatusRequestEntityTooLarge,
		},
		{
			name: "not found",
			err:  errors.Join(ErrNotFound, ErrAlreadyExists, ErrValidation),
			want: http.StatusNotFound,
		},
		{
			name: "already exists",
			err:  errors.Join(ErrAlreadyExists, ErrValidation),
			want: http.StatusConflict,
		},
		{name: "validation", err: ErrValidation, want: http.StatusUnprocessableEntity},
		{name: "unclassified", err: errors.New("boom"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Status(tt.err, http.StatusUnprocessableEntity); got != tt.want {
				t.Errorf("Status() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStatusUsesTheCallSiteValidationStatus(t *testing.T) {
	if got := Status(ErrValidation, http.StatusBadRequest); got != http.StatusBadRequest {
		t.Errorf("Status(ErrValidation, 400) = %d, want 400", got)
	}
	if got := Status(ErrValidation, http.StatusUnprocessableEntity); got != http.StatusUnprocessableEntity {
		t.Errorf("Status(ErrValidation, 422) = %d, want 422", got)
	}
}

func TestWriteJSONWritesBareJSONWithTrailingNewline(t *testing.T) {
	recorder := httptest.NewRecorder()
	value := struct {
		Name string `json:"name"`
	}{Name: "article"}

	if err := WriteJSON(recorder, http.StatusCreated, value); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if recorder.Code != http.StatusCreated {
		t.Errorf("response status = %d, want 201", recorder.Code)
	}
	if got, want := recorder.Body.String(), "{\"name\":\"article\"}\n"; got != want {
		t.Errorf("response body = %q, want %q", got, want)
	}
}

func TestWriteJSONWritesProblemOnMarshalFailure(t *testing.T) {
	recorder := httptest.NewRecorder()

	err := WriteJSON(recorder, http.StatusCreated, make(chan int))
	if err == nil {
		t.Fatal("WriteJSON() error = nil")
	}
	if got, want := err.Error(), "json: unsupported type: chan int"; got != want {
		t.Errorf("WriteJSON() error = %q, want %q", got, want)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", got)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("response status = %d, want 500", recorder.Code)
	}
	wantBody := "{\"type\":\"about:blank\",\"title\":\"Internal Server Error\",\"status\":500," +
		"\"detail\":\"json: unsupported type: chan int\"}\n"
	if got := recorder.Body.String(); got != wantBody {
		t.Errorf("response body = %q, want %q", got, wantBody)
	}
}
