package lp2p

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
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
