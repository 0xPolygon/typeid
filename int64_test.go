package typeid_test

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/typeid"
)

func ExampleNewInt64() {
	id, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	s := id.String()

	prefix, suffix, _ := strings.Cut(s, "_")
	fmt.Println(prefix)
	fmt.Println(len(suffix))
	fmt.Println(id.Int64() > 0)
	// Output:
	// org
	// 13
	// true
}

func ExampleParseInt64() {
	original, _ := typeid.NewInt64[orgPrefix]()
	parsed, err := typeid.ParseInt64[orgPrefix](original.String())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(original == parsed)
	// Output:
	// true
}

func ExampleParseInt64_wrongPrefix() {
	_, err := typeid.ParseInt64[orgPrefix]("foo_0h455vb4pex5v")
	fmt.Println(err)
	// Output:
	// typeid: prefix mismatch: expected "org", got "foo"
}

func ExampleInt64From() {
	id, _ := typeid.NewInt64[orgPrefix]()
	raw := id.Int64()
	reconstructed, err := typeid.Int64From[orgPrefix](raw)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(id == reconstructed)
	// Output:
	// true
}

func ExampleInt64From_rejectsNonPositive() {
	_, err := typeid.Int64From[orgPrefix](-1)
	fmt.Println(err)
	_, err = typeid.Int64From[orgPrefix](0)
	fmt.Println(err)
	// Output:
	// typeid: non-positive Int64
	// typeid: non-positive Int64
}

func ExampleInt64_IsZero() {
	var id OrgID
	fmt.Println(id.IsZero())
	id, _ = typeid.NewInt64[orgPrefix]()
	fmt.Println(id.IsZero())
	// Output:
	// true
	// false
}

func ExampleInt64_json() {
	type Org struct {
		ID   OrgID  `json:"id"`
		Name string `json:"name"`
	}

	id, _ := typeid.NewInt64[orgPrefix]()
	original := Org{ID: id, Name: "Polygon"}
	data, _ := json.Marshal(original)

	var decoded Org
	_ = json.Unmarshal(data, &decoded)
	fmt.Println(original.ID == decoded.ID)
	fmt.Println(strings.Contains(string(data), `"id":"org_`))
	// Output:
	// true
	// true
}

func ExampleInt64_Value() {
	id, _ := typeid.NewInt64[orgPrefix]()
	val, _ := id.Value()
	v, ok := val.(int64)
	fmt.Println(ok)
	fmt.Println(v > 0)
	// Output:
	// true
	// true
}

func ExampleInt64_Scan() {
	id, _ := typeid.NewInt64[orgPrefix]()
	raw := id.Int64()

	var scanned OrgID
	err := scanned.Scan(raw)
	fmt.Println(err == nil)
	fmt.Println(id == scanned)
	// Output:
	// true
	// true
}

func TestInt64_RejectZeroAndNegative(t *testing.T) {
	var zero OrgID

	if _, err := zero.MarshalText(); err == nil {
		t.Error("MarshalText should reject zero")
	}
	if _, err := zero.Value(); err == nil {
		t.Error("Value should reject zero")
	}

	var scanned OrgID
	if err := scanned.Scan(int64(0)); err == nil {
		t.Error("Scan should reject zero")
	}
	if err := scanned.Scan(int64(-1)); err == nil {
		t.Error("Scan should reject negative")
	}
	if err := scanned.Scan(int(-1)); err == nil {
		t.Error("Scan should reject negative int")
	}
}

func TestParseInt64_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"no underscore", "abc"},
		{"suffix too short", "org_abc"},
		{"suffix too long", "org_0h455vb4pex5vv"},
		{"invalid base32 char", "org_0h455vb4pex!v"},
		{"overflow first char", "org_8h455vb4pex5v"},
		{"zero", "org_0000000000000"},
		{"wrong prefix", "user_0h455vb4pex5v"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := typeid.ParseInt64[orgPrefix](tt.input); err == nil {
				t.Errorf("expected error for %q", tt.input)
			}
		})
	}
}

func TestInt64_ScanInvalid(t *testing.T) {
	var id OrgID

	if err := id.Scan("hello"); err == nil {
		t.Error("Scan should reject string")
	}
	if err := id.Scan(true); err == nil {
		t.Error("Scan should reject bool")
	}
	if err := id.Scan(3.14); err == nil {
		t.Error("Scan should reject float64")
	}
}

func TestInt64_GetTime(t *testing.T) {
	before := time.Now()
	id, _ := typeid.NewInt64[orgPrefix]()
	after := time.Now()

	got := id.GetTime()
	if got.Before(before.Truncate(time.Millisecond)) {
		t.Errorf("GetTime %v before creation time %v", got, before)
	}
	if got.After(after.Add(time.Millisecond)) {
		t.Errorf("GetTime %v after creation time %v", got, after)
	}
}

func TestAnyInt64_GetTime(t *testing.T) {
	before := time.Now()
	id, _ := typeid.NewAnyInt64("org")
	after := time.Now()

	got := id.GetTime()
	if got.Before(before.Truncate(time.Millisecond)) {
		t.Errorf("GetTime %v before creation time %v", got, before)
	}
	if got.After(after.Add(time.Millisecond)) {
		t.Errorf("GetTime %v after creation time %v", got, after)
	}
}

func TestInt64_GetTime_KnownVector(t *testing.T) {
	const ms = int64(1700000000000)
	raw := (ms << 15) | 12345
	id, err := typeid.Int64From[orgPrefix](raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := id.GetTime(); !got.Equal(time.UnixMilli(ms)) {
		t.Errorf("GetTime() = %v, want %v", got, time.UnixMilli(ms))
	}
}

func TestInt64_KnownVector(t *testing.T) {
	// timestamp=1700000000000ms, random=12345
	raw := int64(1700000000000<<15) | 12345
	id, err := typeid.Int64From[orgPrefix](raw)
	if err != nil {
		t.Fatal(err)
	}

	const want = "org_01hf7yat00c1s"
	if got := id.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	parsed, err := typeid.ParseInt64[orgPrefix](want)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Int64() != raw {
		t.Errorf("roundtrip Int64 mismatch: got %d, want %d", parsed.Int64(), raw)
	}
}

func BenchmarkInt64_String(b *testing.B) {
	id, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = id.String()
	}
}

func BenchmarkInt64_MarshalText(b *testing.B) {
	id, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		id.MarshalText() //nolint:errcheck
	}
}

func BenchmarkInt64_Parse(b *testing.B) {
	id, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		b.Fatal(err)
	}
	s := id.String()
	b.ResetTimer()
	for b.Loop() {
		typeid.ParseInt64[orgPrefix](s) //nolint:errcheck
	}
}

// ExampleAnyInt64_switchToTypedInt64 narrows [AnyInt64] to [Int64] after a prefix switch.
func ExampleAnyInt64_switchToTypedInt64() {
	const payload = `{"id":"org_01hf7yat00c1s"}`
	type Request struct {
		ID typeid.AnyInt64 `json:"id"`
	}
	var req Request
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		fmt.Println("unmarshal:", err)
		return
	}

	var orgID OrgID
	var err error
	switch req.ID.Prefix() {
	case "org":
		orgID, err = typeid.Int64From[orgPrefix](req.ID.Int64())
	default:
		fmt.Println("unknown prefix")
		return
	}
	if err != nil {
		fmt.Println("narrow:", err)
		return
	}
	fmt.Println(orgID.String())
	// Output:
	// org_01hf7yat00c1s
}

func TestAnyInt64_json(t *testing.T) {
	type Request struct {
		ID typeid.AnyInt64 `json:"id"`
	}

	suffix := "01hf7yat00c1s"
	inputs := []string{
		`{"id":"whatever_` + suffix + `"}`,
		`{"id":"other_prefix_` + suffix + `"}`,
		`{"id":"` + suffix + `"}`,
	}
	for _, raw := range inputs {
		var req Request
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			t.Fatalf("Unmarshal %s: %v", raw, err)
		}
		if req.ID.Int64() <= 0 {
			t.Fatalf("expected positive Int64, got %d", req.ID.Int64())
		}
	}
}

func TestAnyInt64_prefixAndSetPrefix(t *testing.T) {
	suffix := "01hf7yat00c1s"
	id, err := typeid.ParseAnyInt64("foo_" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	if got := id.Prefix(); got != "foo" {
		t.Fatalf("Prefix() = %q, want foo", got)
	}

	id.SetPrefix("bar")
	if got := id.Prefix(); got != "bar" {
		t.Fatalf("after SetPrefix, Prefix() = %q, want bar", got)
	}
	wantText := "bar_" + suffix
	if got, _ := id.MarshalText(); string(got) != wantText {
		t.Fatalf("MarshalText = %q, want %q", got, wantText)
	}
}

func TestAnyInt64_narrowToOrgPrefix(t *testing.T) {
	suffix := "01hf7yat00c1s"
	anyID, err := typeid.ParseAnyInt64("org_" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	var orgID OrgID
	switch anyID.Prefix() {
	case "org":
		orgID, err = typeid.Int64From[orgPrefix](anyID.Int64())
	default:
		t.Fatalf("unexpected prefix %q", anyID.Prefix())
	}
	if err != nil {
		t.Fatal(err)
	}
	if orgID.Int64() != anyID.Int64() {
		t.Errorf("Int64 mismatch")
	}
	if got := orgID.String(); got != "org_"+suffix {
		t.Errorf("String() = %q", got)
	}
}

func TestInt64_Sortable(t *testing.T) {
	a, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	b, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		t.Fatal(err)
	}

	if a.Int64() >= b.Int64() {
		t.Errorf("expected a < b numerically\n  a = %d\n  b = %d", a.Int64(), b.Int64())
	}
	if a.String() >= b.String() {
		t.Errorf("expected a < b lexicographically\n  a = %s\n  b = %s", a, b)
	}
}

func TestInt64_CBOR(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		id, err := typeid.NewInt64[orgPrefix]()
		if err != nil {
			t.Fatal(err)
		}
		data, err := id.MarshalCBOR()
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 11 {
			t.Fatalf("expected 11 bytes, got %d", len(data))
		}
		if data[0] != 0xd8 || data[1] != 0x27 {
			t.Fatalf("expected CBOR tag 39 (0xd8 0x27), got 0x%02x 0x%02x", data[0], data[1])
		}
		var decoded OrgID
		if err := decoded.UnmarshalCBOR(data); err != nil {
			t.Fatal(err)
		}
		if decoded != id {
			t.Errorf("got %s, want %s", decoded, id)
		}
	})

	t.Run("rejects zero", func(t *testing.T) {
		var zero OrgID
		if _, err := zero.MarshalCBOR(); err == nil {
			t.Error("MarshalCBOR should reject zero")
		}
	})

	t.Run("rejects wrong tag", func(t *testing.T) {
		var id OrgID
		if err := id.UnmarshalCBOR([]byte{0xd8, 0x25, 0x1b}); err == nil {
			t.Error("UnmarshalCBOR should reject wrong CBOR tag")
		}
	})

	t.Run("rejects missing tag", func(t *testing.T) {
		var id OrgID
		if err := id.UnmarshalCBOR([]byte{0x1b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}); err == nil {
			t.Error("UnmarshalCBOR should reject untagged data")
		}
	})

	t.Run("rejects truncated payload", func(t *testing.T) {
		// Valid tag 39, valid uint64 header, but only 4 of 8 payload bytes.
		data := []byte{0xd8, 0x27, 0x1b, 0x00, 0x00, 0x00, 0x01}
		var id OrgID
		if err := id.UnmarshalCBOR(data); err == nil {
			t.Error("UnmarshalCBOR should reject truncated payload")
		}
	})

	t.Run("rejects trailing garbage", func(t *testing.T) {
		id, _ := typeid.NewInt64[orgPrefix]()
		data, _ := id.MarshalCBOR()
		data = append(data, 0xff) // append garbage byte
		var decoded OrgID
		if err := decoded.UnmarshalCBOR(data); err == nil {
			t.Error("UnmarshalCBOR should reject trailing bytes")
		}
	})

	t.Run("known value", func(t *testing.T) {
		id, err := typeid.Int64From[orgPrefix](1234567890)
		if err != nil {
			t.Fatal(err)
		}
		data, err := id.MarshalCBOR()
		if err != nil {
			t.Fatal(err)
		}
		var decoded OrgID
		if err := decoded.UnmarshalCBOR(data); err != nil {
			t.Fatal(err)
		}
		if decoded.Int64() != 1234567890 {
			t.Errorf("got %d, want 1234567890", decoded.Int64())
		}
	})

	t.Run("small value", func(t *testing.T) {
		id, err := typeid.Int64From[orgPrefix](42)
		if err != nil {
			t.Fatal(err)
		}
		data, err := id.MarshalCBOR()
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 11 {
			t.Fatalf("expected 11 bytes, got %d", len(data))
		}
		var decoded OrgID
		if err := decoded.UnmarshalCBOR(data); err != nil {
			t.Fatal(err)
		}
		if decoded.Int64() != 42 {
			t.Errorf("got %d, want 42", decoded.Int64())
		}
	})

	t.Run("max int64 roundtrip", func(t *testing.T) {
		id, err := typeid.Int64From[orgPrefix](math.MaxInt64)
		if err != nil {
			t.Fatal(err)
		}
		data, err := id.MarshalCBOR()
		if err != nil {
			t.Fatal(err)
		}
		var decoded OrgID
		if err := decoded.UnmarshalCBOR(data); err != nil {
			t.Fatal(err)
		}
		if decoded.Int64() != math.MaxInt64 {
			t.Errorf("got %d, want %d", decoded.Int64(), int64(math.MaxInt64))
		}
	})

	t.Run("rejects 1<<63", func(t *testing.T) {
		// Craft tag 39 + uint64 with value 1<<63 (overflows int64).
		data := []byte{
			0xd8, 0x27, // tag 39
			0x1b,                                           // uint64
			0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // 1<<63
		}
		var id OrgID
		if err := id.UnmarshalCBOR(data); err == nil {
			t.Error("UnmarshalCBOR should reject 1<<63")
		}
	})

	t.Run("short-form uint widths", func(t *testing.T) {
		tests := []struct {
			name string
			// tag 39 prefix + encoded uint
			data []byte
			want int64
		}{
			{"inline (info<=23)", []byte{0xd8, 0x27, 0x05}, 5},
			{"1-byte (info==24)", []byte{0xd8, 0x27, 0x18, 0x2a}, 42},
			{"2-byte (info==25)", []byte{0xd8, 0x27, 0x19, 0x01, 0x00}, 256},
			{"4-byte (info==26)", []byte{0xd8, 0x27, 0x1a, 0x00, 0x01, 0x00, 0x00}, 65536},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var id OrgID
				if err := id.UnmarshalCBOR(tt.data); err != nil {
					t.Fatalf("UnmarshalCBOR: %v", err)
				}
				if id.Int64() != tt.want {
					t.Errorf("got %d, want %d", id.Int64(), tt.want)
				}
			})
		}
	})
}

func TestAnyInt64_CBOR(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		id, err := typeid.NewAnyInt64("counter")
		if err != nil {
			t.Fatal(err)
		}
		data, err := id.MarshalCBOR()
		if err != nil {
			t.Fatal(err)
		}
		var decoded typeid.AnyInt64
		if err := decoded.UnmarshalCBOR(data); err != nil {
			t.Fatal(err)
		}
		if decoded.Int64() != id.Int64() {
			t.Errorf("value mismatch: got %d, want %d", decoded.Int64(), id.Int64())
		}
	})

	t.Run("rejects zero", func(t *testing.T) {
		var zero typeid.AnyInt64
		if _, err := zero.MarshalCBOR(); err == nil {
			t.Error("MarshalCBOR should reject zero")
		}
	})
}

func BenchmarkInt64_MarshalCBOR(b *testing.B) {
	id, err := typeid.NewInt64[orgPrefix]()
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		id.MarshalCBOR() //nolint:errcheck
	}
}

func FuzzInt64_UnmarshalCBOR(f *testing.F) {
	// Seed with valid tagged encoding.
	id, _ := typeid.NewInt64[orgPrefix]()
	data, _ := id.MarshalCBOR()
	f.Add(data)
	f.Add([]byte{0xd8, 0x27, 0x05})        // tag 39 + inline uint 5
	f.Add([]byte{0xd8, 0x27, 0x18, 0x2a}) // tag 39 + 1-byte uint 42
	f.Add([]byte{})                        // empty

	f.Fuzz(func(t *testing.T, data []byte) {
		var id OrgID
		// Must not panic — errors are fine.
		id.UnmarshalCBOR(data) //nolint:errcheck
	})
}
