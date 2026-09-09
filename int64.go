package typeid

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

const randomBits = 15

// Int64 is a type-safe compact identifier. Maps to Postgres BIGINT.
//
// # Bit layout
//
//	[48-bit unix ms timestamp][15-bit crypto/rand] = 63 bits, always positive
//
// # Timestamp range
//
// 48-bit millisecond timestamp (same as UUIDv7) covers Unix epoch through
// year 10889. No action needed in our lifetimes.
//
// # Collision resistance
//
// 15 random bits leave only 32,768 values per millisecond, so two IDs drawn in
// the same millisecond collide about once per 32k pairs. [NewInt64] closes that
// gap inside a process: it remembers the highest value it has issued and never
// returns that value or a lower one, so a single process cannot produce a
// duplicate no matter how fast it mints.
//
// Across processes the random bits are all that separate two IDs drawn in the
// same millisecond, and the birthday bound still applies: ~R²/65,536,000
// expected collisions per second for R IDs/sec spread over N servers, falling
// as 1/N compared to the same rate on one server. Protect with a UNIQUE
// constraint. For high-throughput resources use [UUID] instead.
//
// # Ordering (sortable)
//
// IDs from one process are strictly increasing, because each value is either a
// fresh timestamp-plus-random draw above the last one or the last one plus one.
// Across processes they are k-sortable: the 48-bit timestamp dominates, so IDs
// sort by creation time at millisecond granularity, and clock skew between
// servers may reorder IDs within the skew window.
//
// Two cases push a value past its own millisecond. A process that mints more
// than 32,768 IDs in one millisecond borrows from the next, and a process whose
// clock steps backwards keeps counting up from where it was. [Int64.GetTime]
// then reports a time slightly ahead of the draw. Both are bounded by how far
// the sequence ran ahead, and both are preferable to handing out a duplicate.
type Int64[P Prefixer] struct {
	val int64
}

// lastInt64 is the highest value this process has issued, shared by [Int64] and
// [AnyInt64] and by every prefix, so that no two of them are ever equal.
var lastInt64 atomic.Int64

// nextInt64 draws a timestamp-plus-random value and lifts it above every value
// this process has already handed out.
func nextInt64() (int64, error) {
	ms := time.Now().UnixMilli()

	var rb [2]byte
	if _, err := rand.Read(rb[:]); err != nil {
		return 0, fmt.Errorf("typeid: crypto/rand: %w", err)
	}
	drawn := (ms << randomBits) | int64(binary.BigEndian.Uint16(rb[:])&0x7FFF)

	for {
		last := lastInt64.Load()
		v := drawn
		if v <= last {
			v = last + 1
		}
		if lastInt64.CompareAndSwap(last, v) {
			return v, nil
		}
	}
}

func NewInt64[P Prefixer]() (Int64[P], error) {
	v, err := nextInt64()
	if err != nil {
		return Int64[P]{}, err
	}
	return Int64[P]{val: v}, nil
}

func Int64From[P Prefixer](v int64) (Int64[P], error) {
	if v <= 0 {
		return Int64[P]{}, ErrNonPositiveInt
	}
	return Int64[P]{val: v}, nil
}

func ParseInt64[P Prefixer](s string) (Int64[P], error) {
	suffix, err := splitTypeid[P](s, Int64SuffixLen)
	if err != nil {
		return Int64[P]{}, err
	}
	v, err := decodeBase32Int64(suffix)
	if err != nil {
		return Int64[P]{}, err
	}
	return Int64From[P](v)
}

func (id Int64[P]) appendText(dst []byte) []byte {
	var p P
	return appendBase32Int64(dst, p.Prefix(), id.val)
}

func (id Int64[P]) String() string {
	var buf [64]byte
	return string(id.appendText(buf[:0]))
}
func (id Int64[P]) Int64() int64 { return id.val }
func (id Int64[P]) IsZero() bool { return id.val == 0 }

// GetTime extracts the millisecond-precision creation timestamp from the
// upper 48 bits of the value.
func (id Int64[P]) GetTime() time.Time { return time.UnixMilli(id.val >> randomBits) }
func (id Int64[P]) MarshalText() ([]byte, error) {
	if id.val <= 0 {
		return nil, ErrNonPositiveInt
	}
	return id.appendText(nil), nil
}

func (id *Int64[P]) UnmarshalText(data []byte) error {
	parsed, err := ParseInt64[P](string(data))
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
func (id Int64[P]) MarshalCBOR() ([]byte, error) {
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

// UnmarshalCBOR decodes CBOR tag 39 wrapping an unsigned integer into the Int64.
func (id *Int64[P]) UnmarshalCBOR(data []byte) error {
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

func (id Int64[P]) Value() (driver.Value, error) {
	if id.val <= 0 {
		return nil, ErrNonPositiveInt
	}
	return id.val, nil
}

func (id *Int64[P]) Scan(src any) error {
	var v int64
	switch sv := src.(type) {
	case int64:
		v = sv
	case int:
		v = int64(sv)
	default:
		return fmt.Errorf("typeid: cannot scan %T into Int64", src)
	}
	if v <= 0 {
		return ErrNonPositiveInt
	}
	id.val = v
	return nil
}
