package lp2p

import (
	"context"
	"errors"
	"fmt"

	"github.com/backkem/go-lp2p/ospc"
)

// uaPeerManager contains anything LP2P related
type uaPeerManager struct {
	ua         *mockUserAgent
	discoverer *ospc.Discoverer

	discoveredAgents []*ospc.RemoteAgent
}

func (m *uaPeerManager) run() {
	var err error
	m.discoverer, err = ospc.Discover()
	if err != nil {
		panic(err) // TODO: Handle
	}

	// TODO: build list of discovered peers.
	agent, err := m.discoverer.Accept(context.Background())
	if err != nil {
		panic(err)
	}

	m.discoveredAgents = []*ospc.RemoteAgent{}
	m.discoveredAgents = append(m.discoveredAgents, agent)
}

// PickAndDial picks a peer form the list and dials it
func (m *uaPeerManager) PickAndDial(localNickname string) (*ospc.Connection, error) {

	// TODO: Render (dynamic) discovered peers & allow user to pick one.
	if len(m.discoveredAgents) < 1 {
		panic("no agent")
	}

	agent := m.discoveredAgents[0]
	conn, err := m.dial(context.Background(), agent, localNickname)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (m *uaPeerManager) authenticatePSK(ctx context.Context, uConn *ospc.UnauthenticatedConnection) (*ospc.Connection, error) {
	role := uConn.GetAuthenticationRole()

	var psk []byte
	var err error
	if role == ospc.AuthenticationRolePresenter {
		if m.ua.PSKOverride != nil {
			psk = m.ua.PSKOverride
		} else {
			psk, err = uConn.GeneratePSK()
			if err != nil {
				return nil, err
			}
		}

		m.present(psk)

	} else {
		err := uConn.RequestAuthenticatePSK()
		if err != nil {
			return nil, err
		}

		psk, err = m.consume()
		if err != nil {
			return nil, err
		}
	}

	conn, err := uConn.AuthenticatePSK(ctx, psk)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (m *uaPeerManager) consentListen(nickname string) error {
	if m.ua.IgnoreConsent {
		return nil
	}

	consent := ""
	fmt.Printf("Accept connections as %s? (y/n):\n", nickname)
	fmt.Scanln(&consent)

	if consent != "y" {
		return errors.New("access denied by user")
	}
	return nil
}

func (m *uaPeerManager) consentAccept(nickname string) error {
	if m.ua.IgnoreConsent {
		return nil
	}
	consent := ""
	fmt.Printf("Accept connection from %s? (y/n):\n", nickname)
	fmt.Scanln(&consent)

	if consent != "y" {
		return errors.New("access denied by user")
	}
	return nil
}

func (m *uaPeerManager) present(psk []byte) {
	m.ua.Presenter(psk)
}

func CLIPresenter(psk []byte) {
	pskEncoded := encodeNumeric(psk)
	fmt.Printf("Pin code: %s\n", pskEncoded)
}

func (m *uaPeerManager) consume() ([]byte, error) {
	return m.ua.Consumer()
}
