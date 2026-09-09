package typeid

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
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
// 15 random bits leave only 32,768 values per millisecond, so two independent
// draws in the same millisecond repeat about once per 32k pairs. [NewInt64] and
// [NewAnyInt64] therefore remember the values this process has already issued in
// the millisecond it is minting into, and draw again on a repeat. A duplicate
// from one process is impossible as a result, and because every id is still a
// uniform draw rather than a value derived from its predecessor, two processes
// are no likelier to collide with each other than they were before.
//
// That leaves the cross-process case, where the random bits are all that
// separate two ids drawn in the same millisecond: ~R²/65,536,000 expected
// collisions per second for R ids/sec across all servers. Keep a UNIQUE
// constraint on the column. For high-throughput resources use [UUID] instead.
//
// # Ordering (k-sortable)
//
// Ids are k-sortable: the 48-bit timestamp in the high bits dominates sort
// order, so ids sort by creation time at millisecond granularity. Two ids from
// the same millisecond are unordered relative to each other, but they cluster on
// the same B-tree leaf pages, so Postgres insert locality is unaffected. Clock
// skew between servers may produce out-of-order ids within that skew window.
//
// Two cases put a value in a millisecond it was not drawn in. A process that
// fills one borrows the next, and a process whose clock steps backwards keeps
// minting into the millisecond it had already reached rather than reusing values
// it has handed out. [Int64.GetTime] then reports a time ahead of the draw. Both
// need conditions you are unlikely to meet, and both beat a duplicate.
type Int64[P Prefixer] struct {
	val int64
}

// nowMilli is a variable so tests can freeze the clock.
var nowMilli = func() int64 { return time.Now().UnixMilli() }

// redrawLimit bounds the work in a single call. Sixteen repeats in a row means
// the millisecond is around 90% spent, and moving to the next one costs less
// than continuing to draw against it.
const redrawLimit = 16

// int64Sequence marks the values already issued in the millisecond this process
// is minting into, one bit per value. [Int64] and [AnyInt64] share one, across
// every prefix, so no two Int64 values from one process are ever equal whatever
// their type.
type int64Sequence struct {
	mu    sync.Mutex
	ms    int64
	used  [1 << randomBits / 64]uint64
	dirty []int64
}

var int64Seq int64Sequence

func (s *int64Sequence) next() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ms := nowMilli(); ms > s.ms {
		s.ms = ms
		s.reset()
	}
	for {
		for range redrawLimit {
			var rb [2]byte
			if _, err := rand.Read(rb[:]); err != nil {
				return 0, fmt.Errorf("typeid: crypto/rand: %w", err)
			}
			r := int64(binary.BigEndian.Uint16(rb[:]) & 0x7FFF)
			if s.used[r/64]&(1<<(r%64)) != 0 {
				continue
			}
			s.used[r/64] |= 1 << (r % 64)
			s.dirty = append(s.dirty, r)
			return s.ms<<randomBits | r, nil
		}
		s.ms++
		s.reset()
	}
}

// reset zeroes only the words a value was actually marked in, so its cost tracks
// how many values the millisecond issued rather than the size of the range.
func (s *int64Sequence) reset() {
	for _, r := range s.dirty {
		s.used[r/64] = 0
	}
	s.dirty = s.dirty[:0]
}

func NewInt64[P Prefixer]() (Int64[P], error) {
	v, err := int64Seq.next()
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
