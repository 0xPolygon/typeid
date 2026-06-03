// Command typeid is a small CLI for generating, encoding, and decoding typeids.
//
// Usage:
//
//	typeid [flags] [input]
//
// Behavior depends on the input:
//
//	(no input)              generate a new typeid
//	<uuid>                  encode a UUID into a typeid
//	<int64>                 encode a decimal int64 value into a short typeid
//	<typeid>                decode a typeid back into its UUID or int64 value
//
// Flags:
//
//	-prefix string   prefix to use when generating or encoding (default none)
//	-type   string   id flavor: "uuid", "int64", or "auto" (default "auto")
//	-format string   output format: "typeid", "uuid", or "int64"
//
// For -type, "auto" means uuid on generation, and on decoding it infers the
// flavor from the suffix length (26 chars = uuid, 13 chars = int64).
//
// For -format, the default depends on the operation: generate and encode emit
// the typeid, while decode emits the underlying UUID or int64 value. Set
// -format to override (e.g. generate an id but print its UUID form).
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/0xPolygon/typeid"
	"github.com/google/uuid"
)

const (
	typeAuto  = "auto"
	typeUUID  = "uuid"
	typeInt64 = "int64"

	formatTypeid = "typeid"

	uuidSuffixLen  = 26
	int64SuffixLen = 13
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("typeid", flag.ContinueOnError)
	prefix := fs.String("prefix", "", "prefix to use when generating or encoding")
	idType := fs.String("type", typeAuto, "id flavor: \"uuid\", \"int64\", or \"auto\"")
	format := fs.String("format", "", "output format: \"typeid\", \"uuid\", or \"int64\" (default depends on operation)")
	fs.Usage = usage(fs)

	// Interleaved parse so flags may appear before or after the positional
	// argument (the stdlib flag package otherwise stops at the first non-flag).
	var rest []string
	pending := args
	for {
		if err := fs.Parse(pending); err != nil {
			return err
		}
		leftover := fs.Args()
		if len(leftover) == 0 {
			break
		}
		rest = append(rest, leftover[0])
		pending = leftover[1:]
	}

	switch *idType {
	case typeAuto, typeUUID, typeInt64:
	default:
		return fmt.Errorf("invalid -type %q (want uuid, int64, or auto)", *idType)
	}
	switch *format {
	case "", formatTypeid, typeUUID, typeInt64:
	default:
		return fmt.Errorf("invalid -format %q (want typeid, uuid, or int64)", *format)
	}

	if len(rest) > 1 {
		return fmt.Errorf("expected at most one argument, got %d", len(rest))
	}

	var res idResult
	var err error
	switch {
	case len(rest) == 0:
		res, err = generate(*prefix, *idType)
	case looksLikeUUID(rest[0]):
		res, err = encodeUUID(*prefix, rest[0])
	case isDecimalInt(rest[0]):
		res, err = encodeInt64(*prefix, rest[0])
	default:
		res, err = decode(rest[0], *idType)
	}
	if err != nil {
		return err
	}

	out, err := res.render(*format)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

// idResult carries the typed value of a single id so any output format can be
// rendered after the operation completes.
type idResult struct {
	flavor     string    // typeUUID or typeInt64
	u          uuid.UUID // valid when flavor == typeUUID
	i          int64     // valid when flavor == typeInt64
	typeidStr  string    // prefixed typeid, e.g. "user_01h4..."
	defaultFmt string    // format used when -format is unset
}

// render returns the id in the requested format, falling back to the
// operation's default.
//
// uuid and int64 share a 48-bit millisecond timestamp layout, so cross-format
// rendering preserves that timestamp: the int64's time and random bits map
// onto a valid UUIDv7 (and back), keeping GetTime consistent across forms.
func (r idResult) render(format string) (string, error) {
	f := format
	if f == "" {
		f = r.defaultFmt
	}
	switch f {
	case formatTypeid:
		return r.typeidStr, nil
	case typeUUID:
		return r.asUUID().String(), nil
	case typeInt64:
		return r.asInt64()
	default:
		return "", fmt.Errorf("invalid -format %q", f)
	}
}

// Int64 bit layout: [48-bit unix-ms timestamp][15-bit random] = 63 bits.
const (
	int64RandomBits = 15
	int64RandomMask = (1 << int64RandomBits) - 1
)

// asUUID returns the id as a uuid.UUID. An int64 is expanded into a valid
// UUIDv7 that carries the same 48-bit timestamp and 15 random bits, so the
// resulting UUID's embedded creation time matches the int64's.
func (r idResult) asUUID() uuid.UUID {
	if r.flavor == typeUUID {
		return r.u
	}
	v := uint64(r.i)
	ms := v >> int64RandomBits     // 48-bit timestamp
	rnd := v & int64RandomMask     // 15 random bits

	var b [16]byte
	// 48-bit timestamp in the first 6 bytes (matches UUIDv7 and the library's GetTime).
	b[0], b[1], b[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	b[3], b[4], b[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	b[6] = 0x70 // version 7, rand_a high nibble = 0
	b[8] = 0x80 // variant 0b10, rand_b high bits = 0
	// Store the 15 random bits in the last two bytes (within rand_b).
	b[14] = byte(rnd >> 8)
	b[15] = byte(rnd)
	return uuid.UUID(b)
}

// asInt64 returns the id as a decimal int64 by recombining the UUID's 48-bit
// timestamp with the 15 random bits stored by asUUID. It rejects any UUID
// whose bits fall outside that layout, since such a UUID carries more entropy
// than an int64 can losslessly hold.
func (r idResult) asInt64() (string, error) {
	if r.flavor == typeInt64 {
		return strconv.FormatInt(r.i, 10), nil
	}
	b := r.u
	// Every bit except the 48-bit timestamp, the version/variant markers, and
	// the 15 low random bits must be zero for the value to fit in an int64.
	if b[6]&0x0F != 0 || b[7] != 0 || b[8]&0x3F != 0 ||
		b[9]|b[10]|b[11]|b[12]|b[13] != 0 || b[14]&0x80 != 0 {
		return "", fmt.Errorf("uuid %s carries more entropy than an int64 can hold", r.u)
	}
	ms := uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])
	rnd := (uint64(b[14])<<8 | uint64(b[15])) & int64RandomMask
	return strconv.FormatInt(int64(ms<<int64RandomBits|rnd), 10), nil
}

// generate creates a new typeid with the given prefix.
func generate(prefix, idType string) (idResult, error) {
	if idType == typeInt64 {
		id, err := typeid.NewAnyInt64(prefix)
		if err != nil {
			return idResult{}, err
		}
		return int64Result(id, formatTypeid), nil
	}
	id, err := typeid.NewAnyUUID(prefix)
	if err != nil {
		return idResult{}, err
	}
	return uuidResult(id, formatTypeid), nil
}

// encodeUUID converts a canonical UUID string into a typeid.
func encodeUUID(prefix, s string) (idResult, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return idResult{}, fmt.Errorf("parse uuid: %w", err)
	}
	id, err := typeid.AnyUUIDFrom(prefix, u)
	if err != nil {
		return idResult{}, err
	}
	return uuidResult(id, formatTypeid), nil
}

// encodeInt64 converts a decimal int64 value into a short typeid.
func encodeInt64(prefix, s string) (idResult, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return idResult{}, fmt.Errorf("parse int64: %w", err)
	}
	id, err := typeid.AnyInt64From(prefix, v)
	if err != nil {
		return idResult{}, err
	}
	return int64Result(id, formatTypeid), nil
}

// decode parses a typeid and returns its underlying UUID or int64 value.
func decode(s, idType string) (idResult, error) {
	flavor := idType
	if flavor == typeAuto {
		var ok bool
		if flavor, ok = inferType(s); !ok {
			return idResult{}, fmt.Errorf("cannot infer type of %q: unexpected suffix length", s)
		}
	}

	if flavor == typeInt64 {
		id, err := typeid.ParseAnyInt64(s)
		if err != nil {
			return idResult{}, err
		}
		return int64Result(id, typeInt64), nil
	}
	id, err := typeid.ParseAnyUUID(s)
	if err != nil {
		return idResult{}, err
	}
	return uuidResult(id, typeUUID), nil
}

func uuidResult(id typeid.AnyUUID, defaultFmt string) idResult {
	return idResult{
		flavor:     typeUUID,
		u:          id.UUID(),
		typeidStr:  id.String(),
		defaultFmt: defaultFmt,
	}
}

func int64Result(id typeid.AnyInt64, defaultFmt string) idResult {
	return idResult{
		flavor:     typeInt64,
		i:          id.Int64(),
		typeidStr:  id.String(),
		defaultFmt: defaultFmt,
	}
}

// inferType guesses the typeid flavor from its base32 suffix length.
func inferType(s string) (string, bool) {
	suffix := s[strings.LastIndex(s, "_")+1:]
	switch len(suffix) {
	case uuidSuffixLen:
		return typeUUID, true
	case int64SuffixLen:
		return typeInt64, true
	default:
		return "", false
	}
}

// isDecimalInt reports whether s is a plain base-10 integer. Such input is
// treated as an int64 value to encode rather than a typeid to decode (a bare
// int64 typeid suffix is base32 and 13 chars; collisions are handled below).
func isDecimalInt(s string) bool {
	if s == "" {
		return false
	}
	rest := strings.TrimPrefix(s, "-")
	if rest == "" {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return false
		}
	}
	return true
}

// looksLikeUUID reports whether s is a canonical hyphenated UUID, which
// distinguishes encode input from a typeid (which never contains hyphens).
func looksLikeUUID(s string) bool {
	if !strings.Contains(s, "-") {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

func usage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprintln(fs.Output(), "Usage: typeid [flags] [input]")
		fmt.Fprintln(fs.Output(), "\n  (no input)   generate a new typeid")
		fmt.Fprintln(fs.Output(), "  <uuid>       encode a UUID into a typeid")
		fmt.Fprintln(fs.Output(), "  <int64>      encode a decimal int64 into a short typeid")
		fmt.Fprintln(fs.Output(), "  <typeid>     decode a typeid into its UUID or int64")
		fmt.Fprintln(fs.Output(), "\nFlags:")
		fs.PrintDefaults()
	}
}
