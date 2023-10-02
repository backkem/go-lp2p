package lp2p

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/backkem/go-lp2p/ospc"
)

// mockUserAgent represents everything a user agent provides to
// the LP2P API.
type mockUserAgent struct {
	pm *uaPeerManager

	IgnoreConsent bool
	PSKOverride   []byte
	Consumer      consumer
	Presenter     presenter
}

type presenter func(psk []byte)
type consumer func() ([]byte, error)

// PeerManager
func (a *mockUserAgent) PeerManager() *uaPeerManager {
	if a.pm == nil {
		a.pm = &uaPeerManager{
			ua: a,
		}

		// Start early
		a.pm.run()
	}
	return a.pm
}

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

type uaPeerListener struct {
	m        *uaPeerManager
	listener *ospc.Listener
}

// Listen starts the OSPC listener
func (m *uaPeerManager) Listen(nickname string) (*uaPeerListener, error) {
	c := ospc.AgentConfig{
		Nickname: nickname,
	}

	err := m.consentListen(nickname)
	if err != nil {
		return nil, err
	}

	listener, err := ospc.Listen(c)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %s", err)
	}

	return &uaPeerListener{
		m:        m,
		listener: listener,
	}, nil
}

// Accept new connections
func (l *uaPeerListener) Accept(ctx context.Context) (*ospc.Connection, error) {
	uConn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	defer uConn.Close() // Cleanup of not authenticated

	err = l.m.consentAccept(uConn.RemoteConfig().Nickname)
	if err != nil {
		return nil, err
	}

	conn, err := l.m.authenticatePSK(ctx, uConn)
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

// Close the listener
func (l *uaPeerListener) Close() error {
	return l.listener.Close()
}

func (m *uaPeerManager) dial(ctx context.Context, agent *ospc.RemoteAgent, localNickname string) (*ospc.Connection, error) {
	uConn, err := agent.Dial(context.Background(),
		ospc.AgentConfig{
			Nickname: localNickname,
		})
	if err != nil {
		return nil, err
	}
	defer uConn.Close() // Cleanup of not authenticated

	conn, err := m.authenticatePSK(ctx, uConn)
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

func CLICollector() ([]byte, error) {
	pskEncoded := ""
	fmt.Println("Enter pin:")
	fmt.Scanln(&pskEncoded)

	psk, err := decodeNumeric(pskEncoded)
	if err != nil {
		return nil, err
	}

	return psk, nil
}

type pskEncoder func([]byte) string

func encodeNumeric(rawPSK []byte) string {
	num := binary.BigEndian.Uint64(rawPSK)
	baseStr := strconv.FormatUint(num, 10)

	var padding, groupSize int
	if len(baseStr) < 9 {
		padding = 3 - (len(baseStr) % 3)
		groupSize = 3
	} else {
		padding = 4 - (len(baseStr) % 4)
		groupSize = 4
	}

	// Zero-pad N on the left
	paddedN := fmt.Sprintf("%0*s", padding+len(baseStr), baseStr)

	// Output N in groups of groupSize digits separated by dashes
	var sb strings.Builder
	for i, ch := range paddedN {
		if i > 0 && i%groupSize == 0 {
			sb.WriteString("-")
		}
		sb.WriteRune(ch)
	}
	return sb.String()
}

type pskDecoder func(string) ([]byte, error)

func decodeNumeric(pks string) ([]byte, error) {
	pks = strings.ReplaceAll(pks, "-", "")
	pks = strings.TrimLeft(pks, "0")

	n, err := strconv.ParseUint(pks, 10, 64)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)

	return buf, nil
}
