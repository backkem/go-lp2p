package ospc

import (
	"bytes"
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

	dec := cbor.NewDecoder(r)
	err = dec.Decode(msg)
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
func DecodeCBOR(r io.Reader, v interface{}) error {
	dec := cbor.NewDecoder(r)
	return dec.Decode(v)
}
