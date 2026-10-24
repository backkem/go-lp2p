package cast

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// The types in this file mirror openscreen cast/common/channel/proto/
// cast_channel.proto (proto2, package openscreen.cast.proto). Field numbers
// and enum values must stay identical to that file.

// ProtocolVersion is CastMessage.ProtocolVersion.
type ProtocolVersion int32

const (
	CastV2_1_0 ProtocolVersion = 0
	CastV2_1_1 ProtocolVersion = 1 // message chunking support (deprecated)
	CastV2_1_2 ProtocolVersion = 2 // reworked message chunking
	CastV2_1_3 ProtocolVersion = 3 // binary payload over utf8
)

// PayloadType is CastMessage.PayloadType.
type PayloadType int32

const (
	PayloadString PayloadType = 0
	PayloadBinary PayloadType = 1
)

// CastMessage is the unit exchanged on a Cast channel.
type CastMessage struct {
	ProtocolVersion ProtocolVersion // field 1, required
	SourceID        string          // field 2, required
	DestinationID   string          // field 3, required
	Namespace       string          // field 4, required
	PayloadType     PayloadType     // field 5, required
	PayloadUTF8     *string         // field 6, optional
	PayloadBinary   []byte          // field 7, optional (nil = absent)
	Continued       *bool           // field 8, optional
	RemainingLength *uint32         // field 9, optional
}

// NewStringMessage builds a CASTV2_1_0 message with a UTF-8 payload.
func NewStringMessage(source, destination, namespace, payload string) *CastMessage {
	return &CastMessage{
		ProtocolVersion: CastV2_1_0,
		SourceID:        source,
		DestinationID:   destination,
		Namespace:       namespace,
		PayloadType:     PayloadString,
		PayloadUTF8:     &payload,
	}
}

// NewBinaryMessage builds a CASTV2_1_0 message with a binary payload.
func NewBinaryMessage(source, destination, namespace string, payload []byte) *CastMessage {
	if payload == nil {
		payload = []byte{}
	}
	return &CastMessage{
		ProtocolVersion: CastV2_1_0,
		SourceID:        source,
		DestinationID:   destination,
		Namespace:       namespace,
		PayloadType:     PayloadBinary,
		PayloadBinary:   payload,
	}
}

// Payload returns the UTF-8 payload or "" if absent.
func (m *CastMessage) Payload() string {
	if m.PayloadUTF8 == nil {
		return ""
	}
	return *m.PayloadUTF8
}

// Marshal serializes m in protobuf wire format, fields in ascending order as
// the C++ protobuf-lite serializer emits them.
func (m *CastMessage) Marshal() []byte { return m.AppendMarshal(nil) }

// AppendMarshal appends the serialized form of m to b.
func (m *CastMessage) AppendMarshal(b []byte) []byte {
	b = appendVarintField(b, 1, protoEnum(int32(m.ProtocolVersion)))
	b = appendStringField(b, 2, m.SourceID)
	b = appendStringField(b, 3, m.DestinationID)
	b = appendStringField(b, 4, m.Namespace)
	b = appendVarintField(b, 5, protoEnum(int32(m.PayloadType)))
	if m.PayloadUTF8 != nil {
		b = appendStringField(b, 6, *m.PayloadUTF8)
	}
	if m.PayloadBinary != nil {
		b = appendBytesField(b, 7, m.PayloadBinary)
	}
	if m.Continued != nil {
		b = appendBoolField(b, 8, *m.Continued)
	}
	if m.RemainingLength != nil {
		b = appendVarintField(b, 9, uint64(*m.RemainingLength))
	}
	return b
}

// ErrMissingRequiredField is returned when a proto2 required field is absent.
var ErrMissingRequiredField = errors.New("cast: missing required protobuf field")

// Unmarshal parses b into m. Unknown fields are skipped. All proto2 required
// fields must be present.
func (m *CastMessage) Unmarshal(b []byte) error {
	*m = CastMessage{}
	var seen uint
	err := rangeFields(b, func(f field) error {
		switch f.num {
		case 1, 5, 8, 9:
			if err := f.expect(wireVarint); err != nil {
				return err
			}
		case 2, 3, 4, 6, 7:
			if err := f.expect(wireBytes); err != nil {
				return err
			}
		default:
			return nil
		}
		seen |= 1 << f.num
		switch f.num {
		case 1:
			m.ProtocolVersion = ProtocolVersion(int32(f.u))
		case 2:
			m.SourceID = string(f.bytes)
		case 3:
			m.DestinationID = string(f.bytes)
		case 4:
			m.Namespace = string(f.bytes)
		case 5:
			m.PayloadType = PayloadType(int32(f.u))
		case 6:
			if !utf8.Valid(f.bytes) {
				return errors.New("cast: payload_utf8 is not valid UTF-8")
			}
			s := string(f.bytes)
			m.PayloadUTF8 = &s
		case 7:
			m.PayloadBinary = append([]byte{}, f.bytes...)
		case 8:
			v := f.u != 0
			m.Continued = &v
		case 9:
			v := uint32(f.u)
			m.RemainingLength = &v
		}
		return nil
	})
	if err != nil {
		return err
	}
	const required = 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<5
	if seen&required != required {
		return ErrMissingRequiredField
	}
	return nil
}

func (m *CastMessage) String() string {
	var payload string
	if m.PayloadType == PayloadBinary {
		payload = fmt.Sprintf("<%d bytes>", len(m.PayloadBinary))
	} else {
		payload = m.Payload()
	}
	return fmt.Sprintf("CastMessage{%s -> %s ns=%s %s}", m.SourceID, m.DestinationID, m.Namespace, payload)
}

// SignatureAlgorithm is the top-level SignatureAlgorithm enum.
type SignatureAlgorithm int32

const (
	SignatureUnspecified    SignatureAlgorithm = 0
	SignatureRSASSAPKCS1v15 SignatureAlgorithm = 1
	SignatureRSASSAPSS      SignatureAlgorithm = 2
)

// HashAlgorithm is the top-level HashAlgorithm enum.
type HashAlgorithm int32

const (
	HashSHA1   HashAlgorithm = 0
	HashSHA256 HashAlgorithm = 1
)

// AuthChallenge is sent by a sender to request device authentication.
// Nil pointer fields are absent and take the proto defaults
// (RSASSA_PKCS1v15, SHA1).
type AuthChallenge struct {
	SignatureAlgorithm *SignatureAlgorithm // field 1
	SenderNonce        []byte              // field 2
	HashAlgorithm      *HashAlgorithm      // field 3
}

// Hash returns the effective hash algorithm (default SHA1).
func (c *AuthChallenge) Hash() HashAlgorithm {
	if c == nil || c.HashAlgorithm == nil {
		return HashSHA1
	}
	return *c.HashAlgorithm
}

// AuthResponse is the receiver's reply to an AuthChallenge.
type AuthResponse struct {
	Signature               []byte              // field 1, required
	ClientAuthCertificate   []byte              // field 2, required (DER)
	IntermediateCertificate [][]byte            // field 3, repeated (DER)
	SignatureAlgorithm      *SignatureAlgorithm // field 4
	SenderNonce             []byte              // field 5
	HashAlgorithm           *HashAlgorithm      // field 6
	CRL                     []byte              // field 7
}

// Hash returns the effective hash algorithm (default SHA1).
func (r *AuthResponse) Hash() HashAlgorithm {
	if r == nil || r.HashAlgorithm == nil {
		return HashSHA1
	}
	return *r.HashAlgorithm
}

// AuthErrorType is AuthError.ErrorType.
type AuthErrorType int32

const (
	AuthErrorInternal                      AuthErrorType = 0
	AuthErrorNoTLS                         AuthErrorType = 1
	AuthErrorSignatureAlgorithmUnavailable AuthErrorType = 2
)

// AuthError reports why a receiver could not answer a challenge.
type AuthError struct {
	ErrorType AuthErrorType // field 1, required
}

// DeviceAuthMessage is the binary payload on the deviceauth namespace.
type DeviceAuthMessage struct {
	Challenge *AuthChallenge // field 1
	Response  *AuthResponse  // field 2
	Error     *AuthError     // field 3
}

// Marshal serializes the message in protobuf wire format.
func (m *DeviceAuthMessage) Marshal() []byte {
	var b []byte
	if c := m.Challenge; c != nil {
		var cb []byte
		if c.SignatureAlgorithm != nil {
			cb = appendVarintField(cb, 1, protoEnum(int32(*c.SignatureAlgorithm)))
		}
		if c.SenderNonce != nil {
			cb = appendBytesField(cb, 2, c.SenderNonce)
		}
		if c.HashAlgorithm != nil {
			cb = appendVarintField(cb, 3, protoEnum(int32(*c.HashAlgorithm)))
		}
		b = appendBytesField(b, 1, cb)
	}
	if r := m.Response; r != nil {
		var rb []byte
		rb = appendBytesField(rb, 1, r.Signature)
		rb = appendBytesField(rb, 2, r.ClientAuthCertificate)
		for _, ic := range r.IntermediateCertificate {
			rb = appendBytesField(rb, 3, ic)
		}
		if r.SignatureAlgorithm != nil {
			rb = appendVarintField(rb, 4, protoEnum(int32(*r.SignatureAlgorithm)))
		}
		if r.SenderNonce != nil {
			rb = appendBytesField(rb, 5, r.SenderNonce)
		}
		if r.HashAlgorithm != nil {
			rb = appendVarintField(rb, 6, protoEnum(int32(*r.HashAlgorithm)))
		}
		if r.CRL != nil {
			rb = appendBytesField(rb, 7, r.CRL)
		}
		b = appendBytesField(b, 2, rb)
	}
	if e := m.Error; e != nil {
		b = appendBytesField(b, 3, appendVarintField(nil, 1, protoEnum(int32(e.ErrorType))))
	}
	return b
}

func cloneBytes(b []byte) []byte { return append([]byte{}, b...) }

// Unmarshal parses b into m.
func (m *DeviceAuthMessage) Unmarshal(b []byte) error {
	*m = DeviceAuthMessage{}
	return rangeFields(b, func(f field) error {
		switch f.num {
		case 1, 2, 3:
			if err := f.expect(wireBytes); err != nil {
				return err
			}
		default:
			return nil
		}
		switch f.num {
		case 1:
			c := &AuthChallenge{}
			if err := c.unmarshal(f.bytes); err != nil {
				return err
			}
			m.Challenge = c
		case 2:
			r := &AuthResponse{}
			if err := r.unmarshal(f.bytes); err != nil {
				return err
			}
			m.Response = r
		case 3:
			e := &AuthError{}
			var seen bool
			err := rangeFields(f.bytes, func(g field) error {
				if g.num != 1 {
					return nil
				}
				if err := g.expect(wireVarint); err != nil {
					return err
				}
				e.ErrorType, seen = AuthErrorType(int32(g.u)), true
				return nil
			})
			if err != nil {
				return err
			}
			if !seen {
				return ErrMissingRequiredField
			}
			m.Error = e
		}
		return nil
	})
}

func (c *AuthChallenge) unmarshal(b []byte) error {
	return rangeFields(b, func(f field) error {
		switch f.num {
		case 1:
			if err := f.expect(wireVarint); err != nil {
				return err
			}
			v := SignatureAlgorithm(int32(f.u))
			c.SignatureAlgorithm = &v
		case 2:
			if err := f.expect(wireBytes); err != nil {
				return err
			}
			c.SenderNonce = cloneBytes(f.bytes)
		case 3:
			if err := f.expect(wireVarint); err != nil {
				return err
			}
			v := HashAlgorithm(int32(f.u))
			c.HashAlgorithm = &v
		}
		return nil
	})
}

func (r *AuthResponse) unmarshal(b []byte) error {
	var seen uint
	err := rangeFields(b, func(f field) error {
		switch f.num {
		case 1, 2, 3, 5, 7:
			if err := f.expect(wireBytes); err != nil {
				return err
			}
		case 4, 6:
			if err := f.expect(wireVarint); err != nil {
				return err
			}
		default:
			return nil
		}
		seen |= 1 << f.num
		switch f.num {
		case 1:
			r.Signature = cloneBytes(f.bytes)
		case 2:
			r.ClientAuthCertificate = cloneBytes(f.bytes)
		case 3:
			r.IntermediateCertificate = append(r.IntermediateCertificate, cloneBytes(f.bytes))
		case 4:
			v := SignatureAlgorithm(int32(f.u))
			r.SignatureAlgorithm = &v
		case 5:
			r.SenderNonce = cloneBytes(f.bytes)
		case 6:
			v := HashAlgorithm(int32(f.u))
			r.HashAlgorithm = &v
		case 7:
			r.CRL = cloneBytes(f.bytes)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if seen&(1<<1|1<<2) != 1<<1|1<<2 {
		return ErrMissingRequiredField
	}
	return nil
}
