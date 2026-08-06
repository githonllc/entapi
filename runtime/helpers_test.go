package entdomain

import (
	"testing"
	"time"
)

func TestPtr(t *testing.T) {
	// int
	i := 42
	p := Ptr(i)
	if *p != 42 {
		t.Errorf("Ptr(42) = %d, want 42", *p)
	}

	// string
	s := "hello"
	ps := Ptr(s)
	if *ps != "hello" {
		t.Errorf("Ptr(\"hello\") = %q, want \"hello\"", *ps)
	}

	// bool
	b := true
	pb := Ptr(b)
	if *pb != true {
		t.Errorf("Ptr(true) = %v, want true", *pb)
	}
}

func TestPtrOrNil(t *testing.T) {
	t.Run("non-empty string returns pointer", func(t *testing.T) {
		got := PtrOrNil("hello")
		if got == nil {
			t.Fatal("PtrOrNil(\"hello\") = nil, want non-nil")
		}
		if *got != "hello" {
			t.Errorf("PtrOrNil(\"hello\") = %q, want \"hello\"", *got)
		}
	})

	t.Run("empty string returns nil", func(t *testing.T) {
		got := PtrOrNil("")
		if got != nil {
			t.Errorf("PtrOrNil(\"\") = %v, want nil", *got)
		}
	})

	t.Run("non-zero time returns pointer", func(t *testing.T) {
		now := time.Now()
		got := PtrOrNil(now)
		if got == nil {
			t.Fatal("PtrOrNil(now) = nil, want non-nil")
		}
		if !got.Equal(now) {
			t.Errorf("PtrOrNil(now) = %v, want %v", *got, now)
		}
	})

	t.Run("zero time returns nil", func(t *testing.T) {
		got := PtrOrNil(time.Time{})
		if got != nil {
			t.Errorf("PtrOrNil(time.Time{}) = %v, want nil", *got)
		}
	})

	t.Run("non-zero int returns pointer", func(t *testing.T) {
		got := PtrOrNil(42)
		if got == nil {
			t.Fatal("PtrOrNil(42) = nil, want non-nil")
		}
		if *got != 42 {
			t.Errorf("PtrOrNil(42) = %d, want 42", *got)
		}
	})

	t.Run("zero int returns nil", func(t *testing.T) {
		got := PtrOrNil(0)
		if got != nil {
			t.Errorf("PtrOrNil(0) = %v, want nil", *got)
		}
	})

	t.Run("true bool returns pointer", func(t *testing.T) {
		got := PtrOrNil(true)
		if got == nil {
			t.Fatal("PtrOrNil(true) = nil, want non-nil")
		}
		if *got != true {
			t.Errorf("PtrOrNil(true) = %v, want true", *got)
		}
	})

	t.Run("false bool returns nil", func(t *testing.T) {
		got := PtrOrNil(false)
		if got != nil {
			t.Errorf("PtrOrNil(false) = %v, want nil", *got)
		}
	})
}
