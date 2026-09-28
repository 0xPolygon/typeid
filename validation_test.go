package typeid

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

type validationProject struct{}

func (validationProject) Prefix() string { return "prj" }

type validationEmpty struct{}

func (validationEmpty) Prefix() string { return "" }

type validationEscaped struct{}

func (validationEscaped) Prefix() string { return "a.b+[x]_(z)?^$|{2}\\/é\n" }

type validationTextID interface {
	StringValidation() StringValidation
	UnmarshalText([]byte) error
}

type validationInput struct {
	Input string `json:"input"`
	Valid bool   `json:"valid"`
}

type validationCorpus struct {
	Name  string            `json:"name"`
	Rules StringValidation  `json:"rules"`
	Cases []validationInput `json:"cases"`
}

func TestStringValidation(t *testing.T) {
	const integer = "00000014sc0pj"
	const uuid = "01kmfjypewe1wrfeb01wjfxand"
	types := []struct {
		name   string
		id     validationTextID
		prefix *string
		seed   string
	}{
		{"int64", &Int64[validationProject]{}, new("prj"), "prj_" + integer},
		{"uuid", &UUID[validationProject]{}, new("prj"), "prj_" + uuid},
		{"any-int64", &AnyInt64{}, nil, "prj_" + integer},
		{"any-uuid", &AnyUUID{}, nil, "prj_" + uuid},
		{"empty-int64", &Int64[validationEmpty]{}, new(""), integer},
		{"empty-uuid", &UUID[validationEmpty]{}, new(""), uuid},
		{"escaped-int64", &Int64[validationEscaped]{}, new(validationEscaped{}.Prefix()), validationEscaped{}.Prefix() + "_" + integer},
		{"escaped-uuid", &UUID[validationEscaped]{}, new(validationEscaped{}.Prefix()), validationEscaped{}.Prefix() + "_" + uuid},
	}
	var corpora []validationCorpus
	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			rules := tc.id.StringValidation()
			if tc.prefix == nil {
				if rules.Prefix != nil {
					t.Fatalf("expected dynamic prefix, got %q", *rules.Prefix)
				}
			} else if rules.Prefix == nil || *rules.Prefix != *tc.prefix {
				t.Fatalf("expected fixed prefix %q, got %v", *tc.prefix, rules.Prefix)
			}
			pattern := regexp.MustCompile("^(?:" + rules.Pattern + ")$")
			corpus := validationCorpus{Name: tc.name, Rules: rules}
			check := func(input string) {
				t.Helper()
				want := tc.id.UnmarshalText([]byte(input)) == nil
				if got := pattern.MatchString(input); got != want {
					t.Fatalf("input %q: pattern=%v UnmarshalText=%v", input, got, want)
				}
				corpus.Cases = append(corpus.Cases, validationInput{input, want})
			}
			if err := tc.id.UnmarshalText([]byte(tc.seed)); err != nil {
				t.Fatalf("invalid seed: %v", err)
			}
			// Populated Any IDs must still describe all prefixes, not their current one.
			if populated := tc.id.StringValidation(); populated.Pattern != rules.Pattern {
				t.Fatal("descriptor depends on the ID value")
			} else if tc.prefix == nil && populated.Prefix != nil {
				t.Fatal("populated dynamic ID reported a fixed prefix")
			}
			check(tc.seed)
			check("")
			for position := range len(tc.seed) {
				// Mutate ASCII bytes only so the JSON corpus preserves valid UTF-8.
				if tc.seed[position] >= 128 {
					continue
				}
				for replacement := range 128 {
					mutated := []byte(tc.seed)
					mutated[position] = byte(replacement)
					check(string(mutated))
				}
				check(tc.seed[:position] + tc.seed[position+1:])
			}
			for _, affix := range []string{" ", "\n", "\r", "\r\n", "\x00", "_", "x", "é", "😀", "\u200b", "\u2028", "\u2029"} {
				check(affix + tc.seed)
				check(tc.seed + affix)
			}
			start := tc.seed[:strings.LastIndex(tc.seed, "_")+1]
			for _, prefix := range []string{start, "", "_", "PRJ_", "usr_", "sk_live_project_", "a\nb_", "😀_"} {
				for _, suffix := range []string{
					integer, strings.ToUpper(integer), uuid, strings.ToUpper(uuid),
					"0000000000000", "0000000000001", "7zzzzzzzzzzzz", "8000000000000", "zzzzzzzzzzzzz",
					"00000000000000000000000000", "0000000000e000000000000000", "8000000000e000000000000000",
				} {
					check(prefix + suffix)
				}
			}
			// Exercise each possible first nonzero digit in an integer suffix.
			for position := range Int64SuffixLen {
				for _, digit := range alphabet {
					suffix := []byte(strings.Repeat("0", Int64SuffixLen))
					suffix[position] = byte(digit)
					check(start + string(suffix))
				}
			}
			// The version and variant occupy characters 10 and 13 respectively.
			for _, version := range alphabet {
				for _, variant := range alphabet {
					suffix := []byte(uuid)
					suffix[10], suffix[13] = byte(version), byte(variant)
					check(start + string(suffix))
				}
			}
			rng := rand.New(rand.NewPCG(1, 2))
			for range 10000 {
				suffix := make([]byte, len(tc.seed)-len(start))
				for i := range suffix {
					suffix[i] = alphabet[rng.IntN(len(alphabet))]
				}
				check(start + string(suffix))
				check(start + strings.ToUpper(string(suffix)))
			}
			corpora = append(corpora, corpus)
		})
	}
	t.Run("javascript", func(t *testing.T) {
		node, err := exec.LookPath("node")
		if err != nil {
			t.Skip("Node.js is required for cross-language parity; make test requires it")
		}
		data, err := json.Marshal(corpora)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(node, "testdata/string-validation.mjs")
		cmd.Stdin = bytes.NewReader(data)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("JavaScript parity failed: %v\n%s", err, output)
		}
		t.Log(string(output))
	})
}
