package application

import (
	"errors"
	"testing"
)

func TestPresentationID(t *testing.T) {
	for i := 0; i < 10; i++ {
		if id := NewPresentationID(); ValidatePresentationID(id) != nil {
			t.Fatalf("NewPresentationID() = %q is invalid", id)
		}
	}
	if err := ValidatePresentationID("0123456789abcdef"); err != nil {
		t.Errorf("16 characters: %v", err)
	}
	for _, bad := range []string{"", "go-iop-12345678", "0123456789abcde\x00", "0123456789abcdé"} {
		if err := ValidatePresentationID(bad); !errors.Is(err, ErrInvalidPresentationID) {
			t.Errorf("ValidatePresentationID(%q) = %v, want ErrInvalidPresentationID", bad, err)
		}
	}
}
