// This file is hand-written. Generation owns the package around it.
package httpdemoent

import (
	"fmt"
	"slices"
	"testing"
)

func ExampleArticleCreateRequestTags() {
	fmt.Println(ArticleCreateRequestTags())
	// Output: [title rank slug internal_note]
}

func ExampleArticlePatchRequestTags() {
	fmt.Println(ArticlePatchRequestTags())
	// Output: [title rank internal_note]
}

func TestRequestTagsAccessorsReturnFreshCopies(t *testing.T) {
	tests := []struct {
		name     string
		accessor func() []string
		source   []string
	}{
		{name: "ArticleCreate", accessor: ArticleCreateRequestTags, source: articleCreateRequestTags},
		{name: "ArticlePatch", accessor: ArticlePatchRequestTags, source: articlePatchRequestTags},
		{name: "AuditLogCreate", accessor: AuditLogCreateRequestTags, source: auditLogCreateRequestTags},
		{name: "AuditLogPatch", accessor: AuditLogPatchRequestTags, source: auditLogPatchRequestTags},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := append([]string(nil), test.source...)
			got := test.accessor()
			if got == nil {
				t.Fatal("accessor returned a nil slice")
			}
			if len(got) == 0 {
				t.Fatal("accessor returned an empty slice")
			}
			if !slices.Equal(got, want) {
				t.Fatalf("accessor returned %q; want %q", got, want)
			}

			got[0] += "-mutated"
			if second := test.accessor(); !slices.Equal(second, want) {
				t.Fatalf("mutating one result changed the next call: got %q; want %q", second, want)
			}
			if !slices.Equal(test.source, want) {
				t.Fatalf("mutating accessor result changed the source: got %q; want %q", test.source, want)
			}
		})
	}
}
