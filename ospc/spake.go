package ospc

import (
	"errors"
)

// TODO: Actual PAKE based on openscreenprotocol#235 and openscreenprotocol#242
type spakeState struct {
}

type spakeSecret struct {
}

func newSpakeClient(psk []byte) (*spakeState, error) {
	return &spakeState{}, nil
}

func newSpakeServer(psk []byte) (*spakeState, error) {
	return &spakeState{}, nil
}

func (s *spakeState) GetLocalPublic() []byte {
	return []byte("TODO")
}

func (s *spakeState) DeriveSecret(remotePublic []byte) (*spakeSecret, error) {
	return &spakeSecret{}, nil
}

func (s *spakeSecret) DeriveConfirmation() []byte {
	return []byte("TODO")
}

func (s *spakeSecret) Verify(remoteConfirmation []byte) error {
	if string(remoteConfirmation) != "TODO" {
		return errors.New("wrong answer")
	}
	return nil
}
