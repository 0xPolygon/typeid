package typeid

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Prefixer is the constraint for type-safe ID prefixes.
type Prefixer interface {
	Prefix() string
}

var (
	ErrOnlyV7         = errors.New("typeid: only UUIDv7 is supported")
	ErrZeroUUID       = errors.New("typeid: zero UUID")
	ErrNonPositiveInt = errors.New("typeid: non-positive Int64")
	ErrOverflowBase32 = errors.New("typeid: base32 overflow at pos 0")
	ErrOverflowInt64  = errors.New("typeid: value overflows int64")
)

const (
	uuidSuffixLen  = 26 // 130-bit capacity, 128 used
	int64SuffixLen = 13 // 65-bit capacity, 63 used
)

// CBOR constants for hand-encoded marshal/unmarshal.
const (
	cborByteString16 = 0x50 // major type 2 (byte string), length 16
	cborUint64       = 0x1b // major type 0 (unsigned int), 8-byte follow

	// Tag headers (major type 6, 1-byte follow = 0xD8).
	// See https://github.com/lucas-clemente/cbor-specs/blob/master/uuid.md
	// and https://github.com/lucas-clemente/cbor-specs/blob/master/id.md
	cborTagUUID = 0x25 // tag 37: value is a UUID (RFC 4122)
	cborTagID   = 0x27 // tag 39: value has identifier semantics
	cborTag1B   = 0xd8 // major type 6, additional info 24 (1-byte tag follows)
)

// decodeCBORTag strips a 2-byte CBOR tag header (0xD8 <tag>) and returns the
// remaining data. Returns an error if the tag is missing or wrong.
func decodeCBORTag(data []byte, want byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("cbor: truncated tag")
	}
	if data[0] != cborTag1B || data[1] != want {
		return nil, fmt.Errorf("cbor: expected tag 0x%02x, got 0x%02x 0x%02x", want, data[0], data[1])
	}
	return data[2:], nil
}

// decodeCBORByteString extracts the payload from a CBOR byte string.
// Rejects trailing bytes after the byte string.
func decodeCBORByteString(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cbor: truncated byte string")
	}
	if data[0]&0xe0 != 0x40 {
		return nil, fmt.Errorf("cbor: expected byte string, got 0x%02x", data[0])
	}
	info := data[0] & 0x1f
	var offset, length int
	switch {
	case info <= 23:
		length, offset = int(info), 1
	case info == 24:
		if len(data) < 2 {
			return nil, fmt.Errorf("cbor: truncated byte string")
		}
		length, offset = int(data[1]), 2
	default:
		return nil, fmt.Errorf("cbor: unsupported length encoding: %d", info)
	}
	if len(data) != offset+length {
		return nil, fmt.Errorf("cbor: expected %d bytes, got %d", offset+length, len(data))
	}
	return data[offset : offset+length], nil
}

// decodeCBORUint64 decodes a CBOR unsigned integer (major type 0).
// Rejects trailing bytes after the integer.
func decodeCBORUint64(data []byte) (uint64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("cbor: truncated uint")
	}
	if data[0]&0xe0 != 0x00 {
		return 0, fmt.Errorf("cbor: expected unsigned integer, got 0x%02x", data[0])
	}
	info := data[0] & 0x1f
	var want int
	switch {
	case info <= 23:
		want = 1
	case info == 24:
		want = 2
	case info == 25:
		want = 3
	case info == 26:
		want = 5
	case info == 27:
		want = 9
	default:
		return 0, fmt.Errorf("cbor: unsupported uint encoding: %d", info)
	}
	if len(data) != want {
		return 0, fmt.Errorf("cbor: expected %d bytes, got %d", want, len(data))
	}
	switch {
	case info <= 23:
		return uint64(info), nil
	case info == 24:
		return uint64(data[1]), nil
	case info == 25:
		return uint64(binary.BigEndian.Uint16(data[1:3])), nil
	case info == 26:
		return uint64(binary.BigEndian.Uint32(data[1:5])), nil
	default: // info == 27
		return binary.BigEndian.Uint64(data[1:9]), nil
	}
}

// splitTypeid splits "prefix_<suffix>" from the right using known suffix length.
// Supports underscores in the prefix (e.g. "project_env_<suffix>").
func splitTypeid[P Prefixer](s string, suffixLen int) (string, error) {
	var p P
	want := p.Prefix()
	j := strings.LastIndex(s, "_") + 1 // 0 = bare suffix; else first byte after last '_'
	prefix, suffix := s[:max(0, j-1)], s[j:]
	if len(suffix) != suffixLen {
		return "", fmt.Errorf("typeid: invalid format: %q", s)
	}
	if prefix != want {
		return "", fmt.Errorf("typeid: prefix mismatch: expected %q, got %q", want, prefix)
	}
	return suffix, nil
}
