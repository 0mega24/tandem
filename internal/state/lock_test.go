package state

import (
	"errors"
	"testing"
)

func TestLockIsExclusive(t *testing.T) {
	s := NewStore(t.TempDir())
	unlock, err := s.Lock("feat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock("feat"); !errors.Is(err, ErrBusy) {
		t.Errorf("second Lock = %v, want ErrBusy", err)
	}
	if other, err := s.Lock("other"); err != nil {
		t.Errorf("Lock(other) = %v, want a lock per space", err)
	} else {
		other()
	}
	unlock()
	again, err := s.Lock("feat")
	if err != nil {
		t.Fatalf("Lock after unlock = %v", err)
	}
	again()
}
