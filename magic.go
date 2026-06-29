package erb

import "strings"

// detectMagicComment ports MRI's ERB::Compiler#detect_magic_comment. It scans
// the run of leading comment tags — "<%#...%>" (or "%#...\n" when percent mode
// is on) anchored at the very start of the template — for a coding: or
// frozen_string_literal: directive, honouring the emacs "-*- ... -*-" wrapper.
// It returns the resolved encoding name (defaulting to "UTF-8", the source
// encoding in our pure-Go context) and the frozen-string-literal value ("" when
// absent).
func detectMagicComment(s string, percent bool) (enc string, frozen string) {
	enc = "UTF-8"
	i := 0
	for {
		comment, next, ok := nextMagicComment(s, i, percent)
		if !ok {
			break
		}
		i = next
		comment = stripEmacsWrapper(comment)
		if v, found := scanDirective(comment, "coding"); found {
			enc = canonicalEncoding(v)
		}
		if v, found := scanDirective(comment, "frozen-string-literal"); found {
			frozen = v
		}
	}
	return enc, frozen
}

// nextMagicComment matches one anchored comment tag at position i. In normal
// mode it matches "<%#" ... "%>"; in percent mode it matches both that form and
// a "%#" ... "\n" percent line. It returns the comment body, the index just
// past the tag, and whether a comment matched at i.
func nextMagicComment(s string, i int, percent bool) (body string, next int, ok bool) {
	if strings.HasPrefix(s[i:], "<%#") {
		end := strings.Index(s[i+3:], "%>")
		if end < 0 {
			return "", i, false
		}
		return s[i+3 : i+3+end], i + 3 + end + 2, true
	}
	if percent && strings.HasPrefix(s[i:], "%#") {
		end := strings.IndexByte(s[i+2:], '\n')
		if end < 0 {
			return "", i, false
		}
		return s[i+2 : i+2+end], i + 2 + end + 1, true
	}
	return "", i, false
}

// stripEmacsWrapper reduces an emacs "-*- key: val -*-" comment to its inner
// "key: val" payload, matching MRI's comment[/-\*-\s*([^\s].*?)\s*-\*-$/] step.
func stripEmacsWrapper(comment string) string {
	trimmed := strings.TrimRight(comment, " \t")
	if !strings.HasSuffix(trimmed, "-*-") {
		return comment
	}
	open := strings.Index(comment, "-*-")
	closeIdx := strings.LastIndex(trimmed, "-*-")
	if open < 0 || closeIdx <= open {
		return comment
	}
	inner := comment[open+3 : closeIdx]
	return strings.TrimSpace(inner)
}

// scanDirective looks for `key` (allowing '-' and '_' interchangeably,
// case-insensitively) followed by ":" or "=" and a value made of alnum, '-'
// and '_'. It mirrors MRI's coding/frozen directive regexps.
func scanDirective(comment, key string) (value string, found bool) {
	lc := normalizeKey(comment)
	want := normalizeKey(key)
	idx := strings.Index(lc, want)
	if idx < 0 {
		return "", false
	}
	j := idx + len(want)
	// Skip spaces, then require ':' or '=' (with optional surrounding spaces).
	for j < len(comment) && (comment[j] == ' ' || comment[j] == '\t') {
		j++
	}
	if j >= len(comment) || (comment[j] != ':' && comment[j] != '=') {
		return "", false
	}
	j++
	for j < len(comment) && (comment[j] == ' ' || comment[j] == '\t') {
		j++
	}
	start := j
	for j < len(comment) && isDirectiveValueByte(comment[j]) {
		j++
	}
	if j == start {
		return "", false
	}
	return comment[start:j], true
}

// normalizeKey lower-cases ASCII letters and maps '_' to '-' so directive keys
// match regardless of the underscore/hyphen spelling MRI accepts.
func normalizeKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + ('a' - 'A'))
		case c == '_':
			b.WriteByte('-')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// isDirectiveValueByte reports whether c may appear in a directive value
// (alnum, '-', '_'), matching MRI's [[:alnum:]\-_] value charset.
func isDirectiveValueByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_'
}

// canonicalEncoding normalises an encoding name the way Ruby's Encoding.find
// does for the names ERB templates realistically carry, stripping a trailing
// -mac/-dos/-unix EOL suffix (as MRI does) and upper-casing. Names beyond the
// common set are upper-cased and passed through; the value only feeds the
// emitted "#coding:" comment line.
func canonicalEncoding(name string) string {
	// Strip the -mac/-dos/-unix EOL marker MRI removes before Encoding.find.
	for _, suf := range []string{"-mac", "-dos", "-unix", "-MAC", "-DOS", "-UNIX"} {
		if strings.HasSuffix(name, suf) {
			name = name[:len(name)-len(suf)]
			break
		}
	}
	up := strings.ToUpper(name)
	if canon, ok := encodingCanon[up]; ok {
		return canon
	}
	return up
}

// encodingCanon maps the upper-cased spelling of common encoding aliases to the
// canonical name Ruby's Encoding.find returns (which preserves mixed case for
// some names like Shift_JIS).
var encodingCanon = map[string]string{
	"UTF-8":       "UTF-8",
	"US-ASCII":    "US-ASCII",
	"ASCII":       "US-ASCII",
	"ASCII-8BIT":  "ASCII-8BIT",
	"BINARY":      "ASCII-8BIT",
	"SHIFT_JIS":   "Shift_JIS",
	"SJIS":        "Shift_JIS",
	"EUC-JP":      "EUC-JP",
	"ISO-8859-1":  "ISO-8859-1",
	"ISO-8859-15": "ISO-8859-15",
	"WINDOWS-31J": "Windows-31J",
	"UTF-16":      "UTF-16",
	"UTF-16LE":    "UTF-16LE",
	"UTF-16BE":    "UTF-16BE",
	"UTF-32":      "UTF-32",
}
