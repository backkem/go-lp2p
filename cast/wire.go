package cast

import (
	"errors"
	"fmt"
	"math"
)

// Minimal protobuf (proto2) wire-format helpers. The Cast channel protocol only
// needs varint, length-delimited and (for unknown-field skipping) fixed-width
// fields, so a full protobuf runtime is not required.

const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

var (
	errTruncated = errors.New("cast: truncated protobuf message")
	errOverflow  = errors.New("cast: protobuf varint overflow")
)

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func appendTag(b []byte, field int, wt int) []byte {
	return appendVarint(b, uint64(field)<<3|uint64(wt))
}

func appendVarintField(b []byte, field int, v uint64) []byte {
	return appendVarint(appendTag(b, field, wireVarint), v)
}

func appendBytesField(b []byte, field int, v []byte) []byte {
	b = appendVarint(appendTag(b, field, wireBytes), uint64(len(v)))
	return append(b, v...)
}

func appendStringField(b []byte, field int, v string) []byte {
	b = appendVarint(appendTag(b, field, wireBytes), uint64(len(v)))
	return append(b, v...)
}

func appendBoolField(b []byte, field int, v bool) []byte {
	var x uint64
	if v {
		x = 1
	}
	return appendVarintField(b, field, x)
}

// protoEnum encodes an enum value the way protobuf does: negative values are
// sign-extended to 64 bits.
func protoEnum(v int32) uint64 { return uint64(int64(v)) }

func consumeVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < len(b); i++ {
		if i == 10 {
			return 0, 0, errOverflow
		}
		c := b[i]
		if i == 9 && c > 1 {
			return 0, 0, errOverflow
		}
		v |= uint64(c&0x7f) << (7 * uint(i))
		if c < 0x80 {
			return v, i + 1, nil
		}
	}
	return 0, 0, errTruncated
}

// field is one decoded protobuf field.
type field struct {
	num   int
	wt    int
	u     uint64 // varint / fixed values
	bytes []byte // length-delimited value (aliases input)
}

// rangeFields iterates over the top-level fields in b.
func rangeFields(b []byte, fn func(f field) error) error {
	for len(b) > 0 {
		tag, n, err := consumeVarint(b)
		if err != nil {
			return err
		}
		b = b[n:]
		num := tag >> 3
		if num == 0 || num > math.MaxInt32 {
			return fmt.Errorf("cast: invalid protobuf field number %d", num)
		}
		f := field{num: int(num), wt: int(tag & 7)}
		switch f.wt {
		case wireVarint:
			f.u, n, err = consumeVarint(b)
			if err != nil {
				return err
			}
			b = b[n:]
		case wireFixed64:
			if len(b) < 8 {
				return errTruncated
			}
			for i := 7; i >= 0; i-- {
				f.u = f.u<<8 | uint64(b[i])
			}
			b = b[8:]
		case wireFixed32:
			if len(b) < 4 {
				return errTruncated
			}
			for i := 3; i >= 0; i-- {
				f.u = f.u<<8 | uint64(b[i])
			}
			b = b[4:]
		case wireBytes:
			l, n, err := consumeVarint(b)
			if err != nil {
				return err
			}
			b = b[n:]
			if l > uint64(len(b)) {
				return errTruncated
			}
			f.bytes = b[:l]
			b = b[l:]
		default:
			return fmt.Errorf("cast: unsupported protobuf wire type %d", f.wt)
		}
		if err := fn(f); err != nil {
			return err
		}
	}
	return nil
}

func (f field) expect(wt int) error {
	if f.wt != wt {
		return fmt.Errorf("cast: protobuf field %d has wire type %d, want %d", f.num, f.wt, wt)
	}
	return nil
}
