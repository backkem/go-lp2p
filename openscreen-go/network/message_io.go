package ospc

import (
	"io"

	"github.com/ugorji/go/codec"
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

	h := &codec.CborHandle{}
	dec := codec.NewDecoder(r, h)
	err = dec.Decode(&msg)
	if err != nil {
		return nil, err
	}

	// fmt.Printf("<-- Read %T\n", msg)
	return msg, nil

}

// Write with type/size prefix
func writeMessage(msg interface{}, w io.Writer) error {
	// fmt.Printf("  --> Writing %T\n", msg)
	// defer fmt.Printf("  --> Done writing %T\n", msg)

	// w = newDebugReadWriter(w)
	tKey, err := typeKeyByMessage(msg)
	if err != nil {
		return err
	}

	err = writeTypeKey(tKey, w)
	if err != nil {
		return err
	}
	h := &codec.CborHandle{}
	enc := codec.NewEncoder(w, h)
	return enc.Encode(msg)
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
	h := &codec.CborHandle{}
	enc := codec.NewEncoder(w, h)
	return enc.Encode(v)
}

// DecodeCBOR decodes a CBOR-encoded value from the reader.
func DecodeCBOR(r io.Reader, v interface{}) error {
	h := &codec.CborHandle{}
	dec := codec.NewDecoder(r, h)
	return dec.Decode(v)
}
