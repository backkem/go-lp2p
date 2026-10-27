package ua

import (
	"errors"
	"net/url"
	"strings"
)

// OpaqueOrigin is the serialization of an opaque origin. It never matches
// another origin, not even itself.
const OpaqueOrigin = "null"

// OriginOf returns the ASCII serialization of the origin of rawURL, as
// `scheme://host[:port]` with the default port left out, or OpaqueOrigin for
// a URL whose origin is opaque (for example a data: URL). It fails if rawURL
// doesn't parse. A bare origin such as "https://a.example" is a URL too.
func OriginOf(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	var defaultPort string
	switch scheme {
	case "http", "ws":
		defaultPort = "80"
	case "https", "wss":
		defaultPort = "443"
	case "ftp":
		defaultPort = "21"
	default:
		// Not a special scheme: the origin is opaque.
		return OpaqueOrigin, nil
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("origin: missing host in " + rawURL)
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	origin := scheme + "://" + host
	if port := u.Port(); port != "" && port != defaultPort {
		origin += ":" + port
	}
	return origin, nil
}

// SameOrigin reports whether two serialized origins are the same. Opaque
// origins match nothing.
func SameOrigin(a, b string) bool {
	return a != OpaqueOrigin && a == b
}
