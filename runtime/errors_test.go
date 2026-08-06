package entdomain

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"direct", ErrNotFound, true},
		{"wrapped", fmt.Errorf("person 123: %w", ErrNotFound), true},
		{"double wrapped", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrNotFound)), true},
		{"nil", nil, false},
		{"unrelated", errors.New("something else"), false},
		{"ErrAlreadyExists", ErrAlreadyExists, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAlreadyExists(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"direct", ErrAlreadyExists, true},
		{"wrapped", fmt.Errorf("email taken: %w", ErrAlreadyExists), true},
		{"nil", nil, false},
		{"unrelated", errors.New("something else"), false},
		{"ErrNotFound", ErrNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAlreadyExists(tt.err); got != tt.want {
				t.Errorf("IsAlreadyExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"direct", ErrValidation, true},
		{"wrapped", fmt.Errorf("email format: %w", ErrValidation), true},
		{"nil", nil, false},
		{"unrelated", errors.New("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidation(tt.err); got != tt.want {
				t.Errorf("IsValidation() = %v, want %v", got, tt.want)
			}
		})
	}
}
