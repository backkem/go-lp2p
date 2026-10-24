// Package ospdemointerop holds interop tests between the Go Open Screen
// Protocol implementation (openscreen-go/network) and the osp_demo binary of
// Chromium's Open Screen Library.
//
// The tests are behind the "interop" build tag and need a built osp_demo:
//
//	ninja -C /work/openscreen/out/debug osp_demo
//	OPENSCREEN_OUT_DIR=/work/openscreen/out/debug \
//	  go test -tags interop -v ./openscreen-go/test/osp-demo-interop/
//
// See INTEROP.md for the findings.
package ospdemointerop
