// Package erb is a pure-Go (no cgo) reimplementation of Ruby's ERB template
// compiler — the deterministic, interpreter-independent core of MRI's
// ERB::Compiler. It turns a template string into the Ruby source that, when
// evaluated against a binding, renders the template, matching MRI 4.0.5
// byte-for-byte. It also provides ERB::Util's html_escape / url_encode helpers.
//
// What it is NOT: the final eval(compiled_src, binding) that produces the
// rendered string needs a Ruby interpreter and is deliberately left to the
// consumer (e.g. go-embedded-ruby/rbgo). This package compiles; the host
// evaluates.
//
// The package faithfully ports MRI's lib/erb/compiler.rb: the SimpleScanner
// (default), TrimScanner (">", "<>") and ExplicitScanner ("-") behaviours, the
// "%"-line percent mode, the <%% / %%> literals, the magic-comment detection,
// and the String#dump text encoding (binary path) that MRI emits.
package erb

import (
	"strings"
)

// Mode selects which ERB dialect Compile targets.
type Mode int

const (
	// ModeERB is classic MRI ERB (the default): Compile reproduces
	// ERB.new(str, trim_mode:, eoutvar:) byte-for-byte, honouring TrimMode.
	ModeERB Mode = iota

	// ModeErubi reproduces the erubi gem's Erubi::Engine whitespace/trim
	// semantics (default engine options: trim on, escape off), so consumers that
	// render through erubi — Sinatra, Rails — get byte-identical output. It
	// differs from every classic ERB trim_mode: a standalone "<%= x %>\n" keeps
	// its trailing newline (unlike trim_mode "<>", which trims it), while a line
	// holding only a "<% ... %>" code (or "<%# ... %>" comment) tag is trimmed
	// automatically (unlike the default, and unlike trim_mode "-", which needs
	// the explicit "<%- ... -%>" form). "-%>" / "=%>" chomps the trailing newline
	// on an expression, and "<%==" HTML-escapes. In this mode TrimMode is ignored
	// (erubi has no trim_mode); EOutVar still selects the buffer variable name.
	ModeErubi
)

// Options configures Compile, mirroring the keyword arguments of MRI's
// ERB.new(str, trim_mode:, eoutvar:).
type Options struct {
	// Mode selects the ERB dialect. The zero value, ModeERB, is classic MRI ERB
	// and leaves every existing consumer unchanged; ModeErubi opts in to
	// erubi-compatible output.
	Mode Mode

	// TrimMode is MRI's trim_mode string. The recognised characters are "-",
	// ">", "<>" and "%", and the one- or two-character combinations MRI accepts
	// (e.g. "%-", "%>", "%<>"). An empty string means no trimming. Of the
	// newline-trimming modes only one applies, in MRI's priority order
	// "-" > "<>" > ">"; "%" (percent-line mode) is independent and may combine
	// with any of them.
	TrimMode string

	// EOutVar names the output-buffer local variable used by the compiled
	// source. When empty it defaults to "_erbout", matching MRI.
	EOutVar string
}

// Compile compiles template into the Ruby source that, when eval'd against a
// binding, builds and returns the rendered string. It mirrors MRI's
// ERB::Compiler#compile contract, returning the source and the magic-encoding
// comment line (the "#coding:UTF-8\n" prefix MRI prepends). The returned src
// already includes that prefix, so callers normally eval src directly; the
// separate magicComment is returned for parity with MRI's two-value contract
// and so a host can inspect the detected encoding.
//
// err is non-nil only for genuinely malformed options; well-formed templates
// never fail to compile (matching MRI, which never raises on template syntax —
// the compiled Ruby may raise at eval time, but that is the host's concern).
func Compile(template string, opts Options) (src string, magicComment string, err error) {
	eoutvar := opts.EOutVar
	if eoutvar == "" {
		eoutvar = "_erbout"
	}
	if opts.Mode == ModeErubi {
		return compileErubi(template, eoutvar)
	}
	c := NewCompiler(opts.TrimMode)
	c.EOutVar = eoutvar
	c.PutCmd = eoutvar + ".<<"
	c.InsertCmd = eoutvar + ".<<"
	c.PreCmd = []string{eoutvar + " = +''"}
	c.PostCmd = []string{eoutvar}
	return c.Compile(template)
}

// HTMLEscape replaces the five HTML-significant characters with their entity
// references, matching ERB::Util.html_escape / ERB::Util.h exactly (note "'"
// becomes "&#39;").
func HTMLEscape(s string) string {
	// Fast path: nothing to escape.
	if !strings.ContainsAny(s, "&<>\"'") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// URLEncode percent-encodes every byte except the RFC-3986 unreserved set
// (A-Z a-z 0-9 - _ . ~), upper-casing the hex digits, matching
// ERB::Util.url_encode / ERB::Util.u exactly. It operates on the raw bytes of
// s (as MRI does via String#b), so each byte of a multibyte UTF-8 character is
// percent-encoded individually.
func URLEncode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hexUpper[c>>4])
			b.WriteByte(hexUpper[c&0x0f])
		}
	}
	return b.String()
}

const hexUpper = "0123456789ABCDEF"
