package erb

import "strings"

// Compiler is the porcelain mirror of MRI's ERB::Compiler. It is exposed (in
// addition to the headline Compile function) so a host such as rbgo can drive
// the put/insert/pre/post command wiring exactly as MRI's ERB.new does. Most
// callers should use Compile.
type Compiler struct {
	// PutCmd handles literal text that is appended to the buffer (MRI put_cmd).
	PutCmd string
	// InsertCmd handles a <%= ... %> expression (MRI insert_cmd).
	InsertCmd string
	// PreCmd are statements prepended to the compiled code (MRI pre_cmd).
	PreCmd []string
	// PostCmd are statements appended to the compiled code (MRI post_cmd).
	PostCmd []string
	// EOutVar is the output-buffer variable name (informational; the actual
	// buffer wiring is carried by PutCmd/InsertCmd/PreCmd/PostCmd).
	EOutVar string

	percent  bool   // "%"-line mode active
	trimMode string // normalised newline-trim mode: "", "-", "<>", ">"
}

// NewCompiler constructs a Compiler for the given raw trim_mode string,
// normalising it the way MRI's ERB::Compiler#prepare_trim_mode does. The
// command fields default to MRI's "print"-based defaults; Compile (the headline
// API) overrides them with buffer-appending commands.
func NewCompiler(trimMode string) *Compiler {
	percent, mode := prepareTrimMode(trimMode)
	return &Compiler{
		PutCmd:    "print",
		InsertCmd: "print",
		PreCmd:    nil,
		PostCmd:   nil,
		percent:   percent,
		trimMode:  mode,
	}
}

// prepareTrimMode mirrors ERB::Compiler#prepare_trim_mode for the String case
// (the only case ERB.new(trim_mode:) reaches in MRI 4.0.5). It returns whether
// percent-line mode is on and the single newline-trim mode that wins, in MRI's
// priority order "-" > "<>" > ">".
func prepareTrimMode(mode string) (percent bool, trim string) {
	percent = strings.Contains(mode, "%")
	switch {
	case strings.Contains(mode, "-"):
		return percent, "-"
	case strings.Contains(mode, "<>"):
		return percent, "<>"
	case strings.Contains(mode, ">"):
		return percent, ">"
	default:
		return percent, ""
	}
}

// Compile compiles the template, returning the Ruby source (including the
// "#coding:UTF-8\n" magic prefix) and that magic-comment line separately, in
// parity with MRI's ERB::Compiler#compile two-value return.
func (c *Compiler) Compile(s string) (src string, magicComment string, err error) {
	// MRI: enc = s.encoding; s = s.b. We operate on the raw bytes of the Go
	// string throughout (Go strings are already byte sequences), which is the
	// binary path MRI takes, so text is dumped byte-for-byte.
	enc, frozen := detectMagicComment(s, c.percent)
	out := newBuffer(c, enc, frozen)

	cs := &compileState{compiler: c, out: out}
	sc := makeScanner(s, c.trimMode, c.percent)
	// MRI skips nil and "" tokens (next if token.nil?; next if token == '').
	// Our scanners never yield those — every string token emitted is non-empty
	// (text runs are guarded, markers are fixed non-empty strings) and the only
	// non-string tokens are :cr and PercentLine — so no guard is needed here.
	sc.scan(func(tok token) {
		if sc.stag == "" {
			cs.compileStag(tok, sc)
		} else {
			cs.compileEtag(tok, sc)
		}
	})
	if len(cs.content) > 0 {
		out.addPutCmd(c, cs.content)
	}
	out.close(c)

	magic := "#coding:" + enc + "\n"
	return out.script.String(), magic, nil
}

// compileState carries the buffered literal text run between tokens (MRI's
// Compiler#content).
type compileState struct {
	compiler *Compiler
	out      *buffer
	content  string
}

// compileStag handles a token while not inside a tag (MRI compile_stag).
func (cs *compileState) compileStag(tok token, sc *scanner) {
	switch {
	case tok.kind == tokPercentLine:
		// A PercentLine arrives only at the start of a fresh source line, by
		// which point any preceding literal text has already been flushed by
		// the trailing-newline path, so cs.content is always empty here. (MRI
		// guards add_put_cmd anyway for its scanner; ours cannot reach it.)
		cs.out.push(tok.str)
		cs.out.cr()
	case tok.kind == tokCR:
		cs.out.cr()
	case tok.str == "<%" || tok.str == "<%=" || tok.str == "<%#":
		sc.stag = tok.str
		if len(cs.content) > 0 {
			cs.out.addPutCmd(cs.compiler, cs.content)
		}
		cs.content = ""
	case tok.str == "\n":
		cs.content += "\n"
		cs.out.addPutCmd(cs.compiler, cs.content)
		cs.content = ""
	case tok.str == "<%%":
		cs.content += "<%"
	default:
		cs.content += tok.str
	}
}

// compileEtag handles a token while inside a tag (MRI compile_etag).
func (cs *compileState) compileEtag(tok token, sc *scanner) {
	switch tok.str {
	case "%>":
		cs.compileContent(sc.stag)
		sc.stag = ""
		cs.content = ""
	case "%%>":
		cs.content += "%>"
	default:
		cs.content += tok.str
	}
}

// compileContent emits the buffered tag body for the closing of a tag whose
// start tag was stag (MRI compile_content).
func (cs *compileState) compileContent(stag string) {
	switch stag {
	case "<%":
		if strings.HasSuffix(cs.content, "\n") {
			cs.out.push(cs.content[:len(cs.content)-1])
			cs.out.cr()
		} else {
			cs.out.push(cs.content)
		}
	case "<%=":
		cs.out.addInsertCmd(cs.compiler, cs.content)
	case "<%#":
		// Only adjust line numbering: emit one blank line per newline in body.
		cs.out.push(strings.Repeat("\n", strings.Count(cs.content, "\n")))
	}
}
