package erb

import (
	"regexp"
	"strings"
)

// This file adds the erubi-compatible compilation dialect (Mode == ModeErubi).
//
// Why it exists: MRI's classic ERB and the erubi gem (which Sinatra/Rails use to
// render .erb templates) disagree about whitespace/newline trimming, and no
// single ERB trim_mode reproduces erubi. The clearest example is a standalone
// expression line: for "<%= x %>\n" erubi KEEPS the trailing newline, while
// classic ERB with trim_mode "<>" trims it. erubi also auto-trims lines that
// contain only a code tag ("<% ... %>\n" — no "="), which classic ERB does only
// when you write the explicit "<%- ... -%>" form. See DEFAULT_REGEXP and the
// Engine loop in the erubi gem (lib/erubi.rb); this is a faithful port of that
// loop, emitting into the same buffer-append shape the rest of this package
// uses so the RENDERED output is byte-identical to Erubi::Engine's.
//
// Scope: this reproduces Erubi::Engine's default options (trim: true,
// escape: false). "<%=" appends (expr).to_s unescaped and "<%==" appends the
// HTML-escaped result, exactly as erubi's default engine does.

// erubiRegexp mirrors Erubi::Engine::DEFAULT_REGEXP:
//
//	/<%(={1,2}|-|\#|%)?(.*?)([-=])?%>([ \t]*\r?\n)?/m
//
// Ruby's /m (dot-matches-newline) maps to Go's (?s). Go's regexp submatch
// semantics ("leftmost, then what a backtracking search finds first", honouring
// greedy/non-greedy) match Ruby's for this pattern, so the four capture groups
// — indicator, code, trailing "-"/"=" (tailch), and the "[ \t]*\r?\n" run after
// "%>" (rspace) — line up with erubi's.
var erubiRegexp = regexp.MustCompile(`(?s)<%(={1,2}|-|#|%)?(.*?)([-=])?%>([ \t]*\r?\n)?`)

// compileErubi is the erubi-dialect counterpart of Compiler.Compile. It renders
// byte-identically to Erubi::Engine.new(template).src (default options) once the
// returned source is eval'd against a binding. eoutvar is the output-buffer
// variable name (erubi's :bufvar / :outvar; MRI's :eoutvar).
func compileErubi(template, eoutvar string) (src string, magicComment string, err error) {
	em := &erubiEmitter{eoutvar: eoutvar}
	// Preamble mirrors erubi's "#{bufvar} = #{bufval}" with a mutable buffer.
	// We keep the "#coding:UTF-8" prefix for parity with the classic path's
	// two-value contract; it is inert for the rendered bytes.
	em.b.WriteString("#coding:UTF-8\n")
	em.b.WriteString(eoutvar + " = +''")

	pos := 0
	isBOL := true // erubi's is_bol: are we at the start of a source line?
	for _, m := range erubiRegexp.FindAllStringSubmatchIndex(template, -1) {
		text := template[pos:m[0]]
		pos = m[1]
		indicator, indicatorNil := erubiGroup(template, m, 1)
		code, _ := erubiGroup(template, m, 2)
		tailch, tailchNil := erubiGroup(template, m, 3)
		rspace, rspaceNil := erubiGroup(template, m, 4)

		// ch = indicator[0] (so "==" -> '='); nil when there is no indicator.
		var ch byte
		if !indicatorNil {
			ch = indicator[0]
		}
		isExpr := !indicatorNil && ch == '='

		// lspace: the leading horizontal whitespace of the current line that
		// precedes this tag, captured only for non-expression tags (so an
		// expression never absorbs its indentation). When captured it is spliced
		// out of text. A captured lspace is truthy in erubi even when empty.
		lspace := ""
		lspaceSet := false
		if !isExpr {
			switch {
			case text == "":
				if isBOL {
					lspaceSet = true // lspace = ""
				}
			case text[len(text)-1] == '\n':
				lspaceSet = true // lspace = ""
			default:
				if ri := strings.LastIndexByte(text, '\n'); ri >= 0 {
					if s := text[ri+1:]; isSpacesTabs(s) {
						lspace = s
						lspaceSet = true
						text = text[:ri+1]
					}
				} else if isBOL && isSpacesTabs(text) {
					lspace = text
					lspaceSet = true
					text = ""
				}
			}
		}

		isBOL = !rspaceNil
		em.addText(text)

		switch {
		case isExpr:
			// "<%= x -%>" / "<%= x =%>": a trailing "-"/"=" suppresses rspace,
			// chomping the newline; otherwise rspace is re-emitted as text so the
			// standalone-expression newline is kept (the classic-vs-erubi gap).
			if !tailchNil && tailch != "" {
				rspaceNil = true
			}
			em.addExpression(indicator, code)
			if !rspaceNil {
				em.addText(rspace)
			}
		case indicatorNil || ch == '-':
			// Code tag. When it stands alone on its line (lspace captured AND a
			// trailing newline), the whole line is absorbed into code and emits
			// no text (erubi's trim). Otherwise lspace/rspace are literal text.
			if lspaceSet && !rspaceNil {
				em.addCode(lspace + code + rspace)
			} else {
				if lspaceSet {
					em.addText(lspace)
				}
				em.addCode(code)
				if !rspaceNil {
					em.addText(rspace)
				}
			}
		case ch == '#':
			// Comment tag: emits only newlines (for line-number parity), one per
			// newline in the body plus one for a trimmed trailing newline.
			n := strings.Count(code, "\n")
			if !rspaceNil {
				n++
			}
			if lspaceSet && !rspaceNil {
				em.addCode(strings.Repeat("\n", n))
			} else {
				if lspaceSet {
					em.addText(lspace)
				}
				em.addCode(strings.Repeat("\n", n))
				if !rspaceNil {
					em.addText(rspace)
				}
			}
		default: // ch == '%': escaped literal delimiters, "<%%...%%>" -> "<%...%>".
			tail := ""
			if !tailchNil {
				tail = tailch
			}
			em.addText(lspace + "<%" + code + tail + "%>" + rspace)
		}
	}
	em.addText(template[pos:])
	em.b.WriteString("; " + eoutvar) // postamble: return the buffer
	return em.b.String(), "#coding:UTF-8\n", nil
}

// erubiEmitter accumulates the compiled Ruby source for the erubi dialect. It
// mirrors erubi's Engine#with_buffer (non-chained) append shape, but targets the
// same "eoutvar.<< ..." wiring the classic path emits so both dialects render
// through an identical mechanism.
type erubiEmitter struct {
	b       strings.Builder
	eoutvar string
}

// stmt appends a "; "-separated statement.
func (em *erubiEmitter) stmt(s string) { em.b.WriteString("; " + s) }

// addText appends a literal-text run (erubi's add_text). Empty runs are dropped,
// matching erubi's "return if text.empty?".
func (em *erubiEmitter) addText(text string) {
	if text == "" {
		return
	}
	em.stmt(em.eoutvar + ".<< " + rubyDump(text) + ".freeze")
}

// addCode appends raw Ruby code as its own statement (erubi's add_code). The
// code may carry a trailing newline (from an absorbed standalone line); the
// "; " separators keep it a valid statement sequence either way.
func (em *erubiEmitter) addCode(code string) { em.stmt(code) }

// addExpression appends an expression tag (erubi's add_expression). "<%="
// (indicator "=") appends the unescaped (expr).to_s; "<%==" (indicator "==")
// appends the HTML-escaped result, matching Erubi::Engine's default (escape
// false) where the "==" indicator inverts to escaping. ERB::Util.html_escape is
// byte-for-byte the function erubi calls (::Erubi.h aliases ERB::Escape).
func (em *erubiEmitter) addExpression(indicator, code string) {
	if indicator == "=" {
		em.stmt(em.eoutvar + ".<<((" + code + ").to_s)")
		return
	}
	em.stmt(em.eoutvar + ".<< ERB::Util.html_escape((" + code + "))")
}

// erubiGroup returns capture group n (1-based) of a FindAllStringSubmatchIndex
// match, and whether it did not participate (Ruby nil vs "").
func erubiGroup(s string, m []int, n int) (value string, isNil bool) {
	a, b := m[2*n], m[2*n+1]
	if a < 0 {
		return "", true
	}
	return s[a:b], false
}

// isSpacesTabs reports whether s consists solely of spaces and tabs (Ruby's
// /\A[ \t]*\z/); the empty string qualifies.
func isSpacesTabs(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}
