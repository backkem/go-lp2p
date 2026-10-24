package ospc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

func readTypeKey(r io.Reader) (TypeKey, error) {
	i, err := readVaruint(r)
	return TypeKey(i), err
}

func writeTypeKey(v TypeKey, w io.Writer) error {
	return writeVaruint(uint64(v), w)
}

func readMessage(r io.Reader) (interface{}, error) {
	typeKey, err := readTypeKey(r)
	if err != nil {
		return nil, err
	}
	return readMessageBody(r, typeKey)
}

// readMessageBody reads the CBOR body of a message of which the type key was
// already read.
func readMessageBody(r io.Reader, typeKey TypeKey) (interface{}, error) {
	msg, err := newMessageByType(typeKey)
	if err != nil {
		return nil, err
	}

	err = decodeCBOR(r, msg)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, fmt.Errorf("cbor decode error: %w", err)
	}

	return msg, nil
}

// Write with type/size prefix.
// The entire message (type key + CBOR body) is buffered before writing
// so that it is emitted as a single Write call. This is required when
// each Write opens a new unidirectional QUIC stream.
func writeMessage(msg interface{}, w io.Writer) error {
	tKey, err := typeKeyByMessage(msg)
	if err != nil {
		return err
	}
	return WriteTypedMessage(w, tKey, msg)
}

// msgName returns a short name of a message for logging.
func msgName(msg interface{}) string {
	name := fmt.Sprintf("%T", msg)
	return strings.TrimPrefix(strings.TrimPrefix(name, "*ospc."), "ospc.")
}

// ReadTypeKey reads a variable-length type key from the reader.
func ReadTypeKey(r io.Reader) (TypeKey, error) {
	return readTypeKey(r)
}

// cborEncMode encodes maps and structs with their keys in the core
// deterministic order of RFC 8949 section 4.2.1, i.e. ascending integer keys.
// The spec doesn't require any order, but some decoders, notably Chromium's
// Open Screen Library, reject maps with keys out of order.
//
// Nil slices and maps are encoded as empty arrays and maps rather than null:
// the CDDL types (e.g. `[* text]`) don't allow null.
var cborEncMode = func() cbor.EncMode {
	em, err := cbor.EncOptions{
		Sort:          cbor.SortCoreDeterministic,
		NilContainers: cbor.NilContainerAsEmpty,
	}.EncMode()
	if err != nil {
		panic(err)
	}
	return em
}()

// EncodeCBOR encodes v as CBOR with deterministically ordered map keys.
func EncodeCBOR(v interface{}) ([]byte, error) {
	return cborEncMode.Marshal(v)
}

// WriteTypedMessage writes v, CBOR-encoded with map keys in ascending order,
// prefixed with the type key. The message is emitted with a single Write
// call.
func WriteTypedMessage(w io.Writer, typeKey TypeKey, v interface{}) error {
	var buf bytes.Buffer
	if err := writeTypeKey(typeKey, &buf); err != nil {
		return err
	}
	data, err := cborEncMode.Marshal(v)
	if err != nil {
		return err
	}
	buf.Write(data)
	_, err = w.Write(buf.Bytes())
	return err
}

// ReadTypedMessage reads a message that must have the given type key and
// decodes its body into v.
func ReadTypedMessage(r io.Reader, typeKey TypeKey, v interface{}) error {
	got, err := readTypeKey(r)
	if err != nil {
		return err
	}
	if got != typeKey {
		return fmt.Errorf("unexpected message type %d, expected %d", got, typeKey)
	}
	return decodeCBOR(r, v)
}

// DecodeCBOR decodes a CBOR-encoded value from the reader.
// Exactly one CBOR data item is consumed, so it is safe to call repeatedly on
// a stream carrying multiple messages.
func DecodeCBOR(r io.Reader, v interface{}) error {
	return decodeCBOR(r, v)
}

func decodeCBOR(r io.Reader, v interface{}) error {
	item, err := readCBORItem(r)
	if err != nil {
		return err
	}
	return cbor.Unmarshal(item, v)
}

const (
	// Upper bound on the size of a single string in a message.
	maxCBORStringLen = 64 << 20
	// Upper bound on the nesting depth of a message.
	maxCBORDepth = 32
)

// readCBORItem reads exactly one CBOR data item from r and returns its raw
// encoding. Unlike cbor.Decoder it never reads past the end of the item, so
// the bytes of any following message in the same stream are left untouched.
// Only the framing is checked here, the content is validated on unmarshal.
func readCBORItem(r io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	ib, err := readByte(r)
	if err != nil {
		return nil, err
	}
	err = scanCBORItem(r, buf, ib, 0)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return buf.Bytes(), err
}

// scanCBORItem copies the item starting with initial byte ib from r into buf.
func scanCBORItem(r io.Reader, buf *bytes.Buffer, ib byte, depth int) error {
	if depth > maxCBORDepth {
		return errors.New("cbor: maximum nesting depth exceeded")
	}
	buf.WriteByte(ib)
	major, info := ib>>5, ib&0x1f

	if info == 31 {
		// Indefinite length: items follow until the "break" stop code.
		switch major {
		case 2, 3, 4, 5:
		default:
			return fmt.Errorf("cbor: unexpected indefinite length for major type %d", major)
		}
		for {
			b, err := readByte(r)
			if err != nil {
				return err
			}
			if b == 0xff {
				buf.WriteByte(b)
				return nil
			}
			err = scanCBORItem(r, buf, b, depth+1)
			if err != nil {
				return err
			}
		}
	}

	var arg uint64
	switch {
	case info < 24:
		arg = uint64(info)
	case info <= 27:
		n := 1 << (info - 24)
		var a [8]byte
		err := readBytes(r, a[:n])
		if err != nil {
			return err
		}
		buf.Write(a[:n])
		for _, b := range a[:n] {
			arg = arg<<8 | uint64(b)
		}
	default:
		return fmt.Errorf("cbor: invalid additional information %d", info)
	}

	switch major {
	case 0, 1, 7:
		// Integer, simple value or float: fully contained in the head.
		return nil
	case 2, 3:
		if arg > maxCBORStringLen {
			return fmt.Errorf("cbor: string length %d exceeds limit", arg)
		}
		_, err := io.CopyN(buf, r, int64(arg))
		return err
	case 4, 5:
		count := arg
		if major == 5 {
			count *= 2
		}
		for i := uint64(0); i < count; i++ {
			b, err := readByte(r)
			if err != nil {
				return err
			}
			err = scanCBORItem(r, buf, b, depth+1)
			if err != nil {
				return err
			}
		}
		return nil
	default: // 6: tag, followed by a single item
		b, err := readByte(r)
		if err != nil {
			return err
		}
		return scanCBORItem(r, buf, b, depth+1)
	}
}
