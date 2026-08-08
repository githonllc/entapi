package entapi

import (
	"reflect"
	"testing"
)

func TestAppendEachPreservesRepeatedPredicateOrder(t *testing.T) {
	var got []string
	AppendEach(&got, []int{3, 5}, func(v int) string { return string(rune('0' + v)) })
	if !reflect.DeepEqual(got, []string{"3", "5"}) {
		t.Fatalf("AppendEach = %v, want [3 5]", got)
	}
}

func TestAppendEachSliceMakesOnePredicatePerOccurrence(t *testing.T) {
	var got []int
	AppendEachSlice(&got, [][]int{{1, 2}, {3, 4}}, func(vs ...int) int {
		return vs[0]*10 + vs[1]
	})
	if !reflect.DeepEqual(got, []int{12, 34}) {
		t.Fatalf("AppendEachSlice = %v, want [12 34]", got)
	}
}
