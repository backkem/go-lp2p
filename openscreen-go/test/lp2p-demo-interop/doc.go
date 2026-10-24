// Package lp2pdemointerop holds interop tests of the Local Peer-to-Peer data
// channel extension between the Go implementation (openscreen-go/network) and
// the lp2p_demo binary of the experimental LP2P branch of Chromium's Open
// Screen Library (github.com/backkem/openscreen, lp2p/data-channels).
//
// The tests are behind the "interop" build tag and need a built lp2p_demo:
//
//	ninja -C /work/openscreen/out/debug lp2p_demo
//	OPENSCREEN_OUT_DIR=/work/openscreen/out/debug \
//	  go test -tags interop -v ./openscreen-go/test/lp2p-demo-interop/
//
// The only non-spec options in use are SPAKE2 over P-256 (both sides) and
// lp2p_demo's --skip_data_channel_open_response, because ospc neither sends
// nor waits for data-channel-open-response. TestSpecOpenResponse shows what
// happens without the latter.
package lp2pdemointerop
