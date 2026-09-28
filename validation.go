package typeid

import (
	"regexp"
	"strconv"
	"strings"
)

// StringValidation describes text accepted by an ID's UnmarshalText method.
// Pattern uses regex syntax shared by Go and JavaScript, without flags. Callers
// must require a match covering the entire input; a substring match is not valid.
// Prefix is the fixed prefix without the separator, or nil for dynamic-prefix IDs.
// A non-nil pointer to an empty string denotes a fixed empty prefix.
// Fixed prefixes must be valid UTF-8 to produce a portable regex descriptor.
type StringValidation struct {
	Pattern string  `json:"pattern"`
	Prefix  *string `json:"prefix"`
}

// StringValidation returns the text constraints of this fixed-prefix integer ID.
func (Int64[P]) StringValidation() StringValidation {
	var prefix P
	return fixedValidation(prefix.Prefix(), int64Pattern())
}

// StringValidation returns the text constraints of this fixed-prefix UUIDv7 ID.
func (UUID[P]) StringValidation() StringValidation {
	var prefix P
	return fixedValidation(prefix.Prefix(), uuidPattern())
}

// StringValidation returns the text constraints of an integer ID with any prefix.
func (AnyInt64) StringValidation() StringValidation {
	return StringValidation{Pattern: "(?:[\\s\\S]*_)?" + int64Pattern()}
}

// StringValidation returns the text constraints of a UUIDv7 ID with any prefix.
func (AnyUUID) StringValidation() StringValidation {
	return StringValidation{Pattern: "(?:[\\s\\S]*_)?" + uuidPattern()}
}

func fixedValidation(prefix, suffix string) StringValidation {
	pattern := regexp.QuoteMeta(prefix) + "_"
	if prefix == "" {
		// splitTypeid accepts both the bare suffix and a leading underscore.
		pattern = "_?"
	}
	return StringValidation{Pattern: pattern + suffix, Prefix: new(prefix)}
}

func characterClass(chars string) string {
	return "[" + chars + strings.ToUpper(chars) + "]"
}

func int64Pattern() string {
	chars := characterClass(alphabet)
	nonzero := characterClass(alphabet[1:])
	// The first digit holds three bits. Each alternative requires a nonzero digit.
	alternatives := []string{"[1-7]" + chars + "{" + strconv.Itoa(Int64SuffixLen-1) + "}"}
	for position := 1; position < Int64SuffixLen; position++ {
		alternatives = append(alternatives, "0{"+strconv.Itoa(position)+"}"+nonzero+chars+"{"+strconv.Itoa(Int64SuffixLen-position-1)+"}")
	}
	return "(?:" + strings.Join(alternatives, "|") + ")"
}

func uuidPattern() string {
	chars := characterClass(alphabet)
	// The version nibble occupies the upper four bits of suffix character 10.
	// The parser checks the version but does not constrain the variant bits.
	version := characterClass(alphabet[7<<1 : (7<<1)+2])
	return "[0-7]" + chars + "{9}" + version + chars + "{" + strconv.Itoa(UUIDSuffixLen-11) + "}"
}
