package entapi

import "testing"

func TestTemplateFuncsExposeCoreSelectors(t *testing.T) {
	funcs := templateFuncs()
	expected := []string{
		"createFields", "patchFields", "responseFields", "responseEdges",
		"hasCreateFamily", "edgeJSONKey",
		"isCreatePointer", "isCreateRequired", "isPatchClearable",
		"queryFields", "parseFields", "unfilterableFields", "searchFields", "isSortable",
	}
	for _, name := range expected {
		if _, ok := funcs[name]; !ok {
			t.Errorf("template func %q is not registered", name)
		}
	}
	if _, old := funcs["domainFields"]; old {
		t.Error("retired domainFields remains registered")
	}
}
