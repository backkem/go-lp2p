// Package ua bundles the user agent logic.
package ua

// mockUserAgent represents everything a user agent provides to
// the LP2P API.
type mockUserAgent struct {
	pm *ConnectionManager

	IgnoreConsent bool
	PSKOverride   []byte
	Consumer      consumer
	Presenter     presenter
}

type presenter func(psk []byte)
type consumer func() ([]byte, error)

// PeerManager
func (a *mockUserAgent) PeerManager() *ConnectionManager {
	if a.pm == nil {
		a.pm = NewConnectionManager(a)
		// Start early
		a.pm.run()
	}
	return a.pm
}
