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
