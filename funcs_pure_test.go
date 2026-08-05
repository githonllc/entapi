package entdomain

import (
	"testing"
)

func TestIsComplexFieldType(t *testing.T) {
	tests := []struct {
		name      string
		fieldType string
		want      bool
	}{
		{name: "string is simple", fieldType: "string", want: false},
		{name: "int is simple", fieldType: "int", want: false},
		{name: "bool is simple", fieldType: "bool", want: false},
		{name: "time.Time is simple", fieldType: "time.Time", want: false},
		{name: "[]string is complex", fieldType: "[]string", want: true},
		{name: "map[string]any is complex", fieldType: "map[string]any", want: true},
		{name: "json.RawMessage is complex", fieldType: "json.RawMessage", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComplexFieldType(tt.fieldType)
			if got != tt.want {
				t.Errorf("isComplexFieldType(%q) = %v, want %v", tt.fieldType, got, tt.want)
			}
		})
	}
}
