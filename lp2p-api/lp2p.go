// Package lp2p provides an implementation of the Local Peer-To-Peer API.
// https://wicg.github.io/local-peer-to-peer/
//
// This package contains the API surface itself. The LP2P API also relies
// on the user agent to provide peer management logic. This logic is implemented
// in the useragent package.
package lp2p

import (
	ua "github.com/backkem/go-lp2p/lp2p-api/internal/useragent"
)

var DefaultUserAgent = ua.NewCLIUserAgent()
