package cast

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Framing constants, matching openscreen cast/common/channel/message_framer.
const (
	// HeaderSize is the size of the big-endian uint32 length prefix that
	// precedes every serialized CastMessage on the wire.
	HeaderSize = 4
	// MaxBodySize is the largest serialized CastMessage accepted or produced.
	MaxBodySize = 65536
	// MaxMessageSize is the largest frame (header + body).
	MaxMessageSize = HeaderSize + MaxBodySize
)

// ErrMessageTooLarge is returned when a frame exceeds MaxBodySize.
var ErrMessageTooLarge = errors.New("cast: message exceeds maximum size")

// EncodeFrame serializes msg and prefixes it with its 4-byte big-endian length.
func EncodeFrame(msg *CastMessage) ([]byte, error) {
	buf := make([]byte, HeaderSize, HeaderSize+64+len(msg.Payload())+len(msg.PayloadBinary))
	buf = msg.AppendMarshal(buf)
	body := len(buf) - HeaderSize
	if body > MaxBodySize {
		return nil, fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, body)
	}
	binary.BigEndian.PutUint32(buf, uint32(body))
	return buf, nil
}

// WriteMessage writes one framed CastMessage to w in a single Write call.
func WriteMessage(w io.Writer, msg *CastMessage) error {
	frame, err := EncodeFrame(msg)
	if err != nil {
		return err
	}
	_, err = w.Write(frame)
	return err
}

// ReadMessage reads exactly one framed CastMessage from r. Partial reads are
// handled by io.ReadFull. A frame announcing a body larger than MaxBodySize
// is rejected before the body is read; the stream is then unusable.
func ReadMessage(r io.Reader) (*CastMessage, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxBodySize {
		return nil, fmt.Errorf("%w: header announces %d bytes", ErrMessageTooLarge, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	msg := &CastMessage{}
	if err := msg.Unmarshal(body); err != nil {
		return nil, err
	}
	return msg, nil
}

// TryDecodeFrame attempts to decode one frame from the front of buf, as the
// C++ MessageFramer::TryDeserialize does. It returns the message and the
// number of bytes consumed, or (nil, 0, nil) if buf does not yet hold a
// complete frame.
func TryDecodeFrame(buf []byte) (*CastMessage, int, error) {
	if len(buf) < HeaderSize {
		return nil, 0, nil
	}
	n := binary.BigEndian.Uint32(buf)
	if n > MaxBodySize {
		return nil, 0, fmt.Errorf("%w: header announces %d bytes", ErrMessageTooLarge, n)
	}
	if uint64(len(buf)) < uint64(HeaderSize)+uint64(n) {
		return nil, 0, nil
	}
	msg := &CastMessage{}
	if err := msg.Unmarshal(buf[HeaderSize : HeaderSize+int(n)]); err != nil {
		return nil, 0, err
	}
	return msg, HeaderSize + int(n), nil
}
