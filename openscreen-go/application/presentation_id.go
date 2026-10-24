package application

import (
	"crypto/rand"
	"errors"
	"fmt"
)

// MinPresentationIDLength is the minimum length of a presentation-id.
// application.bs: "The presentation identifier must follow the restrictions
// defined by section 6.1 of the Presentation API, in that it must consist of
// at least 16 ASCII characters."
const MinPresentationIDLength = 16

// ErrInvalidPresentationID is returned for a presentation-id that violates
// application.bs. A receiver answers a presentation-start-request carrying
// one with result invalid-presentation-id (11).
var ErrInvalidPresentationID = errors.New("invalid presentation-id")

// ValidatePresentationID checks that id consists of at least 16 ASCII
// characters. Like Chromium's openscreen, control characters are rejected
// too: the Presentation API generates identifiers from printable characters.
func ValidatePresentationID(id string) error {
	if len(id) < MinPresentationIDLength {
		return fmt.Errorf("%w: %q has %d characters, at least %d required", ErrInvalidPresentationID, id, len(id), MinPresentationIDLength)
	}
	for i := 0; i < len(id); i++ {
		if c := id[i]; c < 0x20 || c > 0x7e {
			return fmt.Errorf("%w: %q has a non-printable or non-ASCII character", ErrInvalidPresentationID, id)
		}
	}
	return nil
}

// NewPresentationID returns a random presentation-id of 26 base32
// characters (128 bits, crypto/rand.Text), which satisfies
// ValidatePresentationID.
func NewPresentationID() string {
	return rand.Text()
}
