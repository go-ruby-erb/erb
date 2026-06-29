package erb

import "strings"

// This file reproduces the leftmost matching the MRI scanners do with their
// regexes, but by hand so the package stays dependency-free and (more
// importantly) avoids re-deriving Ruby regex semantics. Each helper finds the
// earliest delimiter from a given position, returning the literal text before
// it and the delimiter itself.

// nextStart finds, from i, the first start tag (<%% <%= <%# <%) — the
// SimpleScanner outside-tag delimiter set. Bare newlines are NOT delimiters, so
// a literal text run spans newlines. Matching is leftmost; at the same position
// the longest start tag wins (so "<%=" beats "<%").
func nextStart(s string, i int) (text, marker string, next int) {
	for j := i; j < len(s); j++ {
		if m := matchStartTag(s, j); m != "" {
			return s[i:j], m, j + len(m)
		}
	}
	return s[i:], "", len(s)
}

// nextStartOrNewline is nextStart but also treats a bare "\n" as a delimiter —
// the default-TrimScanner outside-tag delimiter set used in percent mode (where
// the source is fed line by line and the trailing newline becomes a "\n" token).
func nextStartOrNewline(s string, i int) (text, marker string, next int) {
	for j := i; j < len(s); j++ {
		if m := matchStartTag(s, j); m != "" {
			return s[i:j], m, j + len(m)
		}
		if s[j] == '\n' {
			return s[i:j], "\n", j + 1
		}
	}
	return s[i:], "", len(s)
}

// nextEnd finds, from i, the first end tag (%%> or %>) — the default-mode
// inside-tag delimiter set.
func nextEnd(s string, i int) (text, marker string, next int) {
	for j := i; j < len(s); j++ {
		if m := matchEndTag(s, j); m != "" {
			return s[i:j], m, j + len(m)
		}
	}
	return s[i:], "", len(s)
}

// matchStartTag returns the longest start tag at position j, or "".
func matchStartTag(s string, j int) string {
	for _, t := range startTags { // ordered longest-first: <%% <%= <%# <%
		if strings.HasPrefix(s[j:], t) {
			return t
		}
	}
	return ""
}

// matchEndTag returns the longest end tag at position j, or "". %%> is tried
// before %> (longest-first).
func matchEndTag(s string, j int) string {
	for _, t := range endTags { // %%> then %>
		if strings.HasPrefix(s[j:], t) {
			return t
		}
	}
	return ""
}

// walkTrim drives the ">"/"<>" trim scanners. Their regex delimiter set adds
// "%>\r?\n" (tried before the plain tags) to the default set. For each step it
// reports the preceding text and the matched delimiter (one of "%>\n",
// "%>\r\n", a start tag, an end tag, or "\n"). Whether to look for start or end
// tags depends on s.stag, mirroring that these scanners interleave both sets in
// one alternation but only the contextually valid one can fire after the
// compiler updates stag.
func walkTrim(line string, s *scanner, step func(text, marker string)) {
	i := 0
	for i < len(line) {
		text, marker, next := nextTrimDelim(line, i, s.stag != "")
		step(text, marker)
		if marker == "" {
			break
		}
		i = next
	}
}

// nextTrimDelim finds the earliest trim-mode delimiter from i. inTag selects
// whether start tags or a bare "\n" are candidates (outside a tag) versus only
// end-of-tag delimiters (inside a tag). "%>\r?\n" is always a candidate inside
// a tag and takes precedence over a plain "%>".
func nextTrimDelim(s string, i int, inTag bool) (text, marker string, next int) {
	for j := i; j < len(s); j++ {
		if inTag {
			if m := matchPercentGtNL(s, j); m != "" {
				return s[i:j], m, j + len(m)
			}
			if m := matchEndTag(s, j); m != "" {
				return s[i:j], m, j + len(m)
			}
		} else {
			if m := matchStartTag(s, j); m != "" {
				return s[i:j], m, j + len(m)
			}
			if s[j] == '\n' {
				return s[i:j], "\n", j + 1
			}
		}
	}
	return s[i:], "", len(s)
}

// matchPercentGtNL matches "%>\n" or "%>\r\n" at j, returning the full match.
func matchPercentGtNL(s string, j int) string {
	if !strings.HasPrefix(s[j:], "%>") {
		return ""
	}
	rest := s[j+2:]
	switch {
	case strings.HasPrefix(rest, "\r\n"):
		return "%>\r\n"
	case strings.HasPrefix(rest, "\n"):
		return "%>\n"
	}
	return ""
}

// nextStartExplicit finds the first delimiter for "-" mode outside a tag:
// "^[ \t]*<%-" (indentation then "<%-" at line start), "<%-", or a default
// start tag. For the two dash-open forms MRI yields the bare "<%" marker
// (discarding the "-" and, for the indented form, the leading blanks), which is
// what the compiler then treats as a normal code start tag.
func nextStartExplicit(s string, i int) (text, marker string, next int) {
	for j := i; j < len(s); j++ {
		// "^[ \t]*<%-": at a line start, optional blanks, then "<%-".
		if atLineStart(s, j) {
			if k := matchIndentedDashOpen(s, j); k > j {
				return s[i:j], "<%", k
			}
		}
		if strings.HasPrefix(s[j:], "<%-") {
			return s[i:j], "<%", j + 3
		}
		if m := matchStartTag(s, j); m != "" {
			return s[i:j], m, j + len(m)
		}
	}
	return s[i:], "", len(s)
}

// matchIndentedDashOpen returns the index past a "[ \t]*<%-" run starting at j
// (which is at a line start), or j if there is none.
func matchIndentedDashOpen(s string, j int) int {
	k := j
	for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
		k++
	}
	if strings.HasPrefix(s[k:], "<%-") {
		return k + 3
	}
	return j
}

// atLineStart reports whether position j begins a line (start of string or
// right after a newline).
func atLineStart(s string, j int) bool {
	return j == 0 || s[j-1] == '\n'
}

// nextEndExplicit finds the first delimiter for "-" mode inside a tag: "-%>",
// "%%>" or "%>". For "-%>" an immediately-following "\r?\n" is consumed and a
// :cr is signalled (consumedCR). The returned marker is always normalised to
// "%>", "%%>" (the compiler-facing tokens).
func nextEndExplicit(s string, i int) (text, marker string, consumedCR bool, next int) {
	for j := i; j < len(s); j++ {
		if strings.HasPrefix(s[j:], "-%>") {
			end := j + 3
			cr := false
			if strings.HasPrefix(s[end:], "\r\n") {
				end += 2
				cr = true
			} else if strings.HasPrefix(s[end:], "\n") {
				end++
				cr = true
			}
			return s[i:j], "%>", cr, end
		}
		if strings.HasPrefix(s[j:], "%%>") {
			return s[i:j], "%%>", false, j + 3
		}
		if strings.HasPrefix(s[j:], "%>") {
			return s[i:j], "%>", false, j + 2
		}
	}
	return s[i:], "", false, len(s)
}

// splitLinesKeepEnd splits s into lines, keeping the trailing newline on each
// line (like Ruby's String#each_line). A final line without a trailing newline
// is returned as-is.
func splitLinesKeepEnd(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// chomp removes a single trailing "\n" or "\r\n" from s (Ruby's String#chomp
// default), used for PercentLine values.
func chomp(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "\n") {
		return s[:len(s)-1]
	}
	return s
}
