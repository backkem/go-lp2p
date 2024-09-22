package spake2

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
)

const (
	// MaxMsgSize is the maximum size of a SPAKE2 message.
	MaxMsgSize = 32
	// MaxKeySize is the maximum amount of key material that SPAKE2 will produce.
	MaxKeySize = 64
)

var (
	ErrValidationFailed = errors.New("validation failed")
)

type Role uint

const (
	// Client or "A" role
	RoleClient = iota + 1
	// Server or "B" role
	RoleServer
)

type State uint

const (
	StateInit State = iota + 1
	StateMsgGenerated
	StateKeyGenerated
)

// https://datatracker.ietf.org/doc/html/rfc9382

//                  A                       B
//                  |                       |
//                  |                       |
//    (compute pA)  |          pA           |
//                  |---------------------->|
//                  |          pB           | (compute pB)
//                  |<----------------------|
//                  |                       |
//                  |   (derive secrets)    |
//                  |                       |
//    (compute cA)  |          cA           |
//                  |---------------------->|
//                  |          cB           | (compute cB)
//                  |                       | (check cA)
//                  |<----------------------|
//    (check cB)    |                       |

// https://github.com/proto-at-block/bitkey/blob/ca1332ebba0d7626a30f32465452c77fef486347/core/crypto/src/spake2.rs#L55

// https://github.com/google/boringssl/blob/229801d4979eaf56cff1bca27de8df64225d8dfa/crypto/curve25519/spake25519.c#L343

// https://chromium-review.googlesource.com/c/openscreen/+/5827077/2/osp/impl/quic/quic_client.cc
// https://chromium-review.googlesource.com/c/openscreen/+/5827077/2/osp/impl/quic/quic_server.cc
//

// https://www.w3.org/TR/openscreenprotocol/#authentication-with-spake2
// - Elliptic curve is edwards25519.
// - Hash function is SHA-256.
// - Key derivation function is HKDF.
// - Message authentication code is HMAC.
// - Password hash function is SHA-512.

type Context struct {
	PrivateKey     [32]byte
	MyMsg          [32]byte
	PasswordScalar [32]byte
	PasswordHash   [64]byte
	MyName         []byte
	MyNameLen      int
	TheirName      []byte
	TheirNameLen   int
	MyRole         Role
	State          State
}

func NewClient(myName, theirName []byte) (*Context, error) {
	return New(RoleClient, myName, theirName)
}

func NewServer(myName, theirName []byte) (*Context, error) {
	return New(RoleServer, myName, theirName)
}

func New(myRole Role, myName, theirName []byte) (*Context, error) {
	ctx := &Context{
		MyRole:    myRole,
		MyName:    myName,
		TheirName: theirName,
	}
	// Add initialization logic, validation, or error handling if needed
	return ctx, nil
}

func NewMsgBuffer() []byte {
	return make([]byte, MaxMsgSize)
}

func serializeBytes(buf *bytes.Buffer, p []byte) {
	ln := uint64(len(p))
	binary.Write(buf, binary.LittleEndian, ln)
	buf.Write(p)
}

func createTT(context []byte, a, b string, m, n, x, y, z, v []byte, w0 []byte) []byte {
	// TT = len(A)  || A
	// || len(B)  || B
	// || len(pA) || pA
	// || len(pB) || pB
	// || len(K)  || K
	// || len(w)  || w

	var buf bytes.Buffer
	serializeBytes(&buf, context)
	serializeBytes(&buf, []byte(a))
	serializeBytes(&buf, []byte(b))

	serializeBytes(&buf, m)
	serializeBytes(&buf, n)
	serializeBytes(&buf, x)
	serializeBytes(&buf, y)
	serializeBytes(&buf, z)
	serializeBytes(&buf, v)
	serializeBytes(&buf, w0)
	return buf.Bytes()
}

func (c *Context) GenerateMsg(out []byte, password []byte) (int, error) {
	// To begin, A picks x randomly and uniformly from the integers in [0,p)
	// and calculates X=x*P and pA=w*M+X. Then, it transmits pA to B.
	privateTmp := make([]byte, 64)
	_, err := rand.Read(privateTmp)
	if err != nil {
		return 0, err
	}

	// TODO: actual implementation

	copy(out, []byte("TODO"))

	return 4, nil
}

func (c *Context) ProcessMsg(outKey []byte, theirMsg []byte) (int, error) {

	// TODO: actual implementation

	if !bytes.Equal(theirMsg, []byte("TODO")) {
		return 0, errors.New("todo")
	}

	return 0, nil
}
