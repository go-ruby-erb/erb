package erb

// tokKind distinguishes the non-string tokens the scanners emit (the :cr symbol
// and PercentLine objects in MRI) from ordinary string tokens.
type tokKind int

const (
	tokString      tokKind = iota // an ordinary string token
	tokCR                         // MRI's :cr symbol (flush the current line)
	tokPercentLine                // MRI's PercentLine object (str holds its value)
)

// token is one unit yielded by a scanner. For tokString and tokPercentLine str
// holds the text; for tokCR str is unused.
type token struct {
	kind tokKind
	str  string
}

func strTok(s string) token { return token{kind: tokString, str: s} }

// startTags / endTags are MRI's DEFAULT_STAGS / DEFAULT_ETAGS.
var (
	startTags = []string{"<%%", "<%=", "<%#", "<%"}
	endTags   = []string{"%%>", "%>"}
	erbStags  = []string{"<%=", "<%#", "<%"} // TrimScanner::ERB_STAG
)

// scanner is the porcelain interface MRI's compiler drives: it walks the source
// emitting tokens, and the compiler feeds back the current start tag via stag so
// the scanner knows whether to look for a start tag or an end tag.
type scanner struct {
	src      string
	trimMode string
	percent  bool
	stag     string // set by the compiler; "" means "outside a tag"
}

// makeScanner mirrors Scanner.make_scanner: it selects the scanning strategy by
// (trimMode, percent). The strategy differences are encoded in scanLine.
func makeScanner(src, trimMode string, percent bool) *scanner {
	return &scanner{src: src, trimMode: trimMode, percent: percent}
}

// scan walks the source, invoking yield for every token. When percent mode is
// on it splits the source into lines first (MRI's percent_line path); otherwise
// it scans the whole source in one pass.
func (s *scanner) scan(yield func(token)) {
	s.stag = ""
	if s.percent {
		for _, line := range splitLinesKeepEnd(s.src) {
			s.percentLine(line, yield)
		}
		return
	}
	s.scanLine(s.src, yield)
}

// percentLine ports TrimScanner#percent_line: a line beginning with "%" is a
// code line (unless we are mid-tag), and "%%" is the escaped literal "%".
func (s *scanner) percentLine(line string, yield func(token)) {
	if s.stag != "" || len(line) == 0 || line[0] != '%' {
		s.scanLine(line, yield)
		return
	}
	line = line[1:] // drop the leading '%'
	if len(line) > 0 && line[0] == '%' {
		s.scanLine(line, yield)
		return
	}
	yield(token{kind: tokPercentLine, str: chomp(line)})
}

// scanLine dispatches to the per-trim-mode scanning routine, mirroring the
// method TrimScanner selects in its initializer (and the StringScanner-based
// SimpleScanner/ExplicitScanner for the strscan-backed modes — the token
// streams are identical, so a single faithful walker covers all cases).
func (s *scanner) scanLine(line string, yield func(token)) {
	switch s.trimMode {
	case ">":
		s.trimLine1(line, yield)
	case "<>":
		s.trimLine2(line, yield)
	case "-":
		s.explicitTrimLine(line, yield)
	default:
		s.plainScan(line, yield)
	}
}

// plainScan implements the default (no newline trimming) tokenisation
// (SimpleScanner). When outside a tag it splits on start tags only — bare
// newlines are NOT delimiters, so a literal text run spans newlines and is
// emitted as a single put_cmd. Inside a tag it splits on end tags.
func (s *scanner) plainScan(line string, yield func(token)) {
	i := 0
	for i < len(line) {
		if s.stag == "" {
			var text, marker string
			var next int
			if s.percent {
				// Percent mode uses the default TrimScanner, whose delimiter
				// set includes a bare "\n" (the source is fed line by line).
				text, marker, next = nextStartOrNewline(line, i)
			} else {
				// SimpleScanner: start tags only; text spans newlines.
				text, marker, next = nextStart(line, i)
			}
			if text != "" {
				yield(strTok(text))
			}
			if marker != "" {
				yield(strTok(marker))
				// The compiler sets s.stag for <%/<%=/<%#; for "<%%" it stays
				// "". Mirror that so the next iteration scans correctly.
				s.applyStagEffect(marker)
			}
			i = next
		} else {
			text, marker, next := nextEnd(line, i)
			if text != "" {
				yield(strTok(text))
			}
			if marker != "" {
				yield(strTok(marker))
				if marker == "%>" {
					s.stag = ""
				}
			}
			i = next
		}
	}
}

// applyStagEffect mirrors the compiler's effect on scanner.stag for the start
// markers, so the scanner's own next step looks for the right delimiter. The
// real compiler sets stag for <%/<%=/<%#; "<%%" and "\n" leave it "".
func (s *scanner) applyStagEffect(marker string) {
	switch marker {
	case "<%", "<%=", "<%#":
		s.stag = marker
	}
}

// trimLine1 ports TrimScanner#trim_line1 (">" mode): a "%>\n" / "%>\r\n" run
// becomes "%>" followed by a :cr token.
func (s *scanner) trimLine1(line string, yield func(token)) {
	s.trimScan(line, yield, func(marker string, yield func(token)) bool {
		if marker == "%>\n" || marker == "%>\r\n" {
			yield(strTok("%>"))
			s.stag = ""
			yield(token{kind: tokCR})
			return true
		}
		return false
	})
}

// trimLine2 ports TrimScanner#trim_line2 ("<>" mode): "%>\n" becomes "%>"
// followed by :cr only when the matched line head was an ERB start tag,
// otherwise "%>" followed by a literal "\n".
func (s *scanner) trimLine2(line string, yield func(token)) {
	head := ""
	headSet := false
	emit := func(t token) {
		if !headSet {
			head = t.str
			headSet = true
		}
		yield(t)
	}
	walkTrim(line, s, func(text, marker string) {
		// In our walker a bare "\n" is always a marker (a delimiter), never a
		// text run, so the head-reset for newlines lives in the marker branch
		// below; text here is only the literal run preceding a delimiter.
		if text != "" {
			emit(strTok(text))
		}
		if marker == "" {
			return
		}
		if marker == "%>\n" || marker == "%>\r\n" {
			yield(strTok("%>"))
			s.stag = ""
			if isErbStag(head) {
				yield(token{kind: tokCR})
			} else {
				yield(strTok("\n"))
			}
			head = ""
			headSet = false
			return
		}
		emit(strTok(marker))
		s.applyStagEffectTrim(marker)
		if marker == "\n" {
			head = ""
			headSet = false
		}
	})
}

// explicitTrimLine ports ExplicitScanner / explicit_trim_line ("-" mode):
// "<%-" (optionally indented) becomes "<%", "-%>\n" becomes "%>" + :cr, and
// "-%>" becomes "%>".
func (s *scanner) explicitTrimLine(line string, yield func(token)) {
	i := 0
	for i < len(line) {
		if s.stag == "" {
			text, marker, next := nextStartExplicit(line, i)
			if text != "" {
				yield(strTok(text))
			}
			if marker != "" {
				yield(strTok(marker))
				s.applyStagEffect(marker)
			}
			i = next
		} else {
			text, marker, consumedCR, next := nextEndExplicit(line, i)
			if text != "" {
				yield(strTok(text))
			}
			if marker != "" {
				yield(strTok(marker))
				if marker == "%>" {
					s.stag = ""
					if consumedCR {
						yield(token{kind: tokCR})
					}
				}
			}
			i = next
		}
	}
}

// applyStagEffectTrim is applyStagEffect for the trim scanners (same effect).
func (s *scanner) applyStagEffectTrim(marker string) { s.applyStagEffect(marker) }

// trimScan is the shared driver for trimLine1: it walks tokens, lets the hook
// handle a "%>\n"/"%>\r\n" run, and otherwise emits text/markers like the
// default scan but with the ">"-mode delimiter set (which includes "%>\r?\n").
func (s *scanner) trimScan(line string, yield func(token), hook func(string, func(token)) bool) {
	walkTrim(line, s, func(text, marker string) {
		if text != "" {
			yield(strTok(text))
		}
		if marker == "" {
			return
		}
		if hook(marker, yield) {
			return
		}
		yield(strTok(marker))
		s.applyStagEffectTrim(marker)
		if marker == "%>" {
			s.stag = ""
		}
	})
}

// isErbStag reports whether s is one of "<%=", "<%#", "<%" (TrimScanner#is_erb_stag?).
func isErbStag(s string) bool {
	for _, t := range erbStags {
		if s == t {
			return true
		}
	}
	return false
}
