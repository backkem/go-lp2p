package ospc

import (
	"bytes"
	"errors"
	"fmt"
	"io"

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
	// r = newDebugReadWriter(r)
	typeKey, err := readTypeKey(r)
	if err != nil {
		return nil, err
	}

	// fmt.Println("Got typeKey", typeKey)

	msg, err := newMessageByType(typeKey)
	if err != nil {
		return nil, err
	}

	err = decodeCBOR(r, msg)
	if err != nil {
		return nil, fmt.Errorf("cbor decode error: %w", err)
	}

	fmt.Printf("<-- Read %T\n", msg)
	return msg, nil

}

// Write with type/size prefix.
// The entire message (type key + CBOR body) is buffered before writing
// so that it is emitted as a single Write call. This is required when
// each Write opens a new unidirectional QUIC stream.
func writeMessage(msg interface{}, w io.Writer) error {
	fmt.Printf("  --> Writing %T\n", msg)

	tKey, err := typeKeyByMessage(msg)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	err = writeTypeKey(tKey, &buf)
	if err != nil {
		return err
	}
	cborData, err := cbor.Marshal(msg)
	if err != nil {
		return err
	}
	buf.Write(cborData)

	_, err = w.Write(buf.Bytes())
	return err
}

// ReadMessage reads a CBOR-encoded message with type key prefix from the reader.
// This is the exported version for use by application protocols.
func ReadMessage(r io.Reader) (interface{}, error) {
	return readMessage(r)
}

// WriteMessage writes a CBOR-encoded message with type key prefix to the writer.
// This is the exported version for use by application protocols.
func WriteMessage(msg interface{}, w io.Writer) error {
	return writeMessage(msg, w)
}

// ReadTypeKey reads a variable-length type key from the reader.
func ReadTypeKey(r io.Reader) (TypeKey, error) {
	return readTypeKey(r)
}

// WriteTypeKey writes a variable-length type key to the writer.
func WriteTypeKey(v TypeKey, w io.Writer) error {
	return writeTypeKey(v, w)
}

// EncodeCBOR encodes a value to CBOR format.
func EncodeCBOR(v interface{}, w io.Writer) error {
	data, err := cbor.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
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
