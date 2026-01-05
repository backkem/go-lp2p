package ua

import (
	"context"
	"errors"
	"fmt"

	"github.com/backkem/go-lp2p/openscreen-go/network"
)

// ConnectionManager manages connections with remote peers.
// Connection needs to pass multiple stages to be granted to an origin:
//  1. Discovery: list the remote peer + gather metadata?
//  2. Authorization: a trust relation (certificate pair) must be established.
//     While user consent is required per origin, the trust relation that is
//     established during authorization can be reused across origins.
//  3. Consent: the user agent must receive user concent to pas a connection to an origin.
//     Upon initial connection, consent should be granted as early as possible
//     ensuring non or bare minimal data is exchanged with a remote peer.
//     Origin grants are specific to a trust relation, in case authentication has
//     to be re-established, all corresponding origin-grants must be revoked.
//     A user can indicate if they want the origin grant to be permanent or 1-time.
type ConnectionManager struct {
	ua         *mockUserAgent
	discoverer *ospc.Discoverer

	// Discovery
	discoveredAgents map[ospc.PeerID]*ospc.DiscoveredAgent
}

func NewConnectionManager(ua *mockUserAgent) *ConnectionManager {
	return &ConnectionManager{
		ua:               ua,
		discoveredAgents: make(map[ospc.PeerID]*ospc.DiscoveredAgent),
	}
}

func (m *ConnectionManager) run() {
	var err error
	m.discoverer, err = ospc.Discover()
	if err != nil {
		panic(err) // TODO: Handle
	}

	// TODO: build list of discovered peers.
	// TODO: how to know when outdated?
	agent, err := m.discoverer.Accept(context.Background())
	if err != nil {
		panic(err)
	}

	m.discoveredAgents[agent.PeerID] = agent
}

// PickAndDial picks a peer form the list and dials it
func (m *ConnectionManager) PickAndDial(localNickname string) (*ospc.Connection, error) {

	// TODO: Render (dynamic) discovered peers & allow user to pick one.
	if len(m.discoveredAgents) < 1 {
		panic("no agent")
	}
	var agent *ospc.DiscoveredAgent
	for _, v := range m.discoveredAgents {
		agent = v // Just pick one for now.
		break
	}

	conn, err := m.dial(context.Background(), agent, localNickname)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (m *ConnectionManager) authenticatePSK(ctx context.Context, uConn *ospc.UnauthenticatedConnection) (*ospc.Connection, error) {
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

func (m *ConnectionManager) consentListen(nickname string) error {
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

func (m *ConnectionManager) consentAccept(nickname string) error {
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

func (m *ConnectionManager) present(psk []byte) {
	m.ua.Presenter(psk)
}

func (m *ConnectionManager) consume() ([]byte, error) {
	return m.ua.Consumer()
}
