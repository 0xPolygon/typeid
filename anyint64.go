package typeid

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// AnyInt64 is a compact identifier with a runtime-configurable prefix.
// Unlike [Int64], the prefix is not fixed at compile time.
type AnyInt64 struct {
	val    int64
	prefix string
}

func NewAnyInt64(prefix string) (AnyInt64, error) {
	ms := time.Now().UnixMilli()

	var rb [2]byte
	if _, err := rand.Read(rb[:]); err != nil {
		return AnyInt64{}, fmt.Errorf("typeid: crypto/rand: %w", err)
	}
	r := int64(binary.BigEndian.Uint16(rb[:]) & 0x7FFF)

	return AnyInt64{val: (ms << randomBits) | r, prefix: prefix}, nil
}

func AnyInt64From(prefix string, v int64) (AnyInt64, error) {
	if v <= 0 {
		return AnyInt64{}, ErrNonPositiveInt
	}
	return AnyInt64{val: v, prefix: prefix}, nil
}

func ParseAnyInt64(s string) (AnyInt64, error) {
	j := strings.LastIndex(s, "_") + 1
	pref, suffix := s[:max(0, j-1)], s[j:]
	if len(suffix) != int64SuffixLen {
		return AnyInt64{}, fmt.Errorf("typeid: invalid format: %q", s)
	}
	v, err := decodeBase32Int64(suffix)
	if err != nil {
		return AnyInt64{}, err
	}
	if v <= 0 {
		return AnyInt64{}, ErrNonPositiveInt
	}
	return AnyInt64{val: v, prefix: pref}, nil
}

func (id AnyInt64) Int64() int64   { return id.val }
func (id AnyInt64) Prefix() string { return id.prefix }
func (id *AnyInt64) SetPrefix(s string) {
	id.prefix = s
}

func (id AnyInt64) appendText(dst []byte) []byte {
	return appendBase32Int64(dst, id.prefix, id.val)
}

func (id AnyInt64) String() string {
	var buf [64]byte
	return string(id.appendText(buf[:0]))
}

func (id AnyInt64) IsZero() bool { return id.val == 0 }

func (id AnyInt64) MarshalText() ([]byte, error) {
	if id.val <= 0 {
		return nil, ErrNonPositiveInt
	}
	return id.appendText(nil), nil
}

func (id *AnyInt64) UnmarshalText(data []byte) error {
	parsed, err := ParseAnyInt64(string(data))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// MarshalCBOR encodes the value as CBOR tag 39 wrapping an unsigned integer.
// Output is always 11 bytes (2-byte tag + 9-byte uint64) — fixed-width by
// design, not RFC 8949 §4.2.1 deterministic encoding. The decoder accepts all
// CBOR unsigned integer widths for interop.
func (id AnyInt64) MarshalCBOR() ([]byte, error) {
	if id.val <= 0 {
		return nil, ErrNonPositiveInt
	}
	out := make([]byte, 11) // 2 (tag) + 1 (header) + 8 (uint64)
	out[0] = cborTag1B
	out[1] = cborTagID
	out[2] = cborUint64
	binary.BigEndian.PutUint64(out[3:], uint64(id.val))
	return out, nil
}

// UnmarshalCBOR decodes CBOR tag 39 wrapping an unsigned integer into the AnyInt64.
// The prefix is not restored — call SetPrefix after unmarshaling if needed.
func (id *AnyInt64) UnmarshalCBOR(data []byte) error {
	inner, err := decodeCBORTag(data, cborTagID)
	if err != nil {
		return fmt.Errorf("typeid: %w", err)
	}
	v, err := decodeCBORUint64(inner)
	if err != nil {
		return fmt.Errorf("typeid: %w", err)
	}
	if v == 0 {
		return ErrNonPositiveInt
	}
	if v > math.MaxInt64 {
		return ErrOverflowInt64
	}
	id.val = int64(v)
	return nil
}

func (id AnyInt64) Value() (driver.Value, error) {
	if id.val <= 0 {
		return nil, ErrNonPositiveInt
	}
	return id.val, nil
}

func (id *AnyInt64) Scan(src any) error {
	var v int64
	switch sv := src.(type) {
	case int64:
		v = sv
	case int:
		v = int64(sv)
	default:
		return fmt.Errorf("typeid: cannot scan %T into AnyInt64", src)
	}
	if v <= 0 {
		return ErrNonPositiveInt
	}
	id.val = v
	return nil
}
