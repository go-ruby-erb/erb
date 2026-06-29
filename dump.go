package erb

import "strings"

// rubyDump reproduces Ruby's String#dump on a binary (ASCII-8BIT) string, which
// is the form MRI's ERB::Compiler#add_put_cmd uses (it calls content.dump after
// s = s.b). The result is a double-quoted Ruby string literal that round-trips
// the exact bytes:
//
//   - " and \ are backslash-escaped;
//   - the C-style escapes \a \b \t \n \v \f \r \e are used;
//   - '#' is escaped to "\#" only when immediately followed by '{', '$' or '@'
//     (otherwise it is emitted literally), matching Ruby's interpolation guard;
//   - other printable ASCII bytes (0x20..0x7E) are emitted literally;
//   - every remaining byte is emitted as \xHH with uppercase hex.
func rubyDump(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\a':
			b.WriteString("\\a")
		case '\b':
			b.WriteString("\\b")
		case '\t':
			b.WriteString("\\t")
		case '\n':
			b.WriteString("\\n")
		case '\v':
			b.WriteString("\\v")
		case '\f':
			b.WriteString("\\f")
		case '\r':
			b.WriteString("\\r")
		case '\x1b': // \e (escape)
			b.WriteString("\\e")
		case '#':
			// Escape only when it would start a Ruby interpolation/variable ref.
			if i+1 < len(s) && (s[i+1] == '{' || s[i+1] == '$' || s[i+1] == '@') {
				b.WriteString("\\#")
			} else {
				b.WriteByte('#')
			}
		default:
			if c >= 0x20 && c <= 0x7e {
				b.WriteByte(c)
			} else {
				b.WriteByte('\\')
				b.WriteByte('x')
				b.WriteByte(hexUpper[c>>4])
				b.WriteByte(hexUpper[c&0x0f])
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
