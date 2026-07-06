package erb

import (
	"os/exec"
	"strings"
	"testing"
)

// erubiCase is a template paired with the Ruby locals needed to render it. The
// same template is compiled here (Mode: ModeErubi) and by the real erubi gem,
// and the two rendered strings are compared byte-for-byte.
type erubiCase struct {
	tpl  string
	vars map[string]string // local name -> ruby literal
}

// erubiCorpus exercises every erubi whitespace/trim edge case plus the tag
// kinds, so that eval(ourErubiSrc) == eval(Erubi::Engine.new(tpl).src). Cases
// with vars render an expression; the rest render with side-effect-free code so
// they need no binding.
var erubiCorpus = []erubiCase{
	// Plain text and trailing newlines.
	{"", nil},
	{"plain text", nil},
	{"trailing newline\n", nil},
	{"two\nlines\n", nil},
	{"tabs\tand 'quotes' and \"dquotes\" and back\\slash\n", nil},
	{"unicode héllo 日本語 😀\n", nil},

	// The headline divergence: a standalone expression line KEEPS its newline.
	{"<%= n %>\n", map[string]string{"n": "1"}},
	{"a\n<%= n %>\nb\n", map[string]string{"n": "2"}},
	{"<%= n %>\n<%= n %>\n", map[string]string{"n": "7"}},
	// Expression chomp forms "-%>" and "=%>".
	{"<%= n -%>\n", map[string]string{"n": "3"}},
	{"<%= n =%>\nafter", map[string]string{"n": "4"}},
	// Expression not on its own line: no trimming either way.
	{"x<%= n %>y\n", map[string]string{"n": "5"}},
	{"  <%= n %>  \n", map[string]string{"n": "6"}}, // rspace not captured (spaces then nl? has nl) -> kept

	// Standalone code lines are auto-trimmed (no "=").
	{"<% x = 1 %>\n", nil},
	{"  <% x = 1 %>\n", nil},
	{"\t<% x = 1 %>\r\n", nil},
	{"<%- x = 1 -%>\n", nil},
	{"top\n<% if true %>\nyes\n<% end %>\nbot\n", nil},
	{"<% 3.times do |i| %>i<% end %>\n", nil},
	// Code tag not alone on its line: whitespace/newline preserved.
	{"x<% y = 1 %>z\n", nil},
	{"<% y = 1 %> tail\n", nil},
	{"a\n<% y = 1 %> b\n", nil},
	{"a\n  <% y = 1 %>\n", nil},
	// Adjacent tags (second tag sees empty text, not at BOL).
	{"<% x = 1 %><% y = 2 %>\n", nil},

	// Comment tags.
	{"<%# a comment %>\nX\n", nil},
	{"x<%# c %>y\n", nil},
	{"x<%# c %>\nY\n", nil},    // comment not at BOL, rspace kept as newline
	{"  <%# c %>after\n", nil}, // comment at BOL with indent, no rspace (trailing text)
	{"<%# multi\nline\ncomment %>\nZ\n", nil},

	// Escaped-delimiter literals "<%%...%%>".
	{"a<%% literal %%>b\n", nil},
	{"<%% x %>\n", nil},
	{"pre<%% a -%>post\n", nil},

	// Escaping: "<%=" is raw, "<%==" HTML-escapes (erubi default engine).
	{"<%= '<b>&\"x\"' %>\n", nil},
	{"<%== '<b>&\"x\"' %>\n", nil},
	{"raw <%= h %> esc <%== h %>\n", map[string]string{"h": `"a<b>c&d'e"`}},
}

func erubiAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("ruby"); err != nil {
		return false
	}
	return exec.Command("ruby", "-e", "require 'erubi'").Run() == nil
}

// TestErubiRenderMatchesErubi proves ModeErubi renders byte-identically to the
// real erubi gem's Erubi::Engine across the corpus.
func TestErubiRenderMatchesErubi(t *testing.T) {
	if !erubiAvailable(t) {
		t.Skip("ruby+erubi not available; skipping erubi differential test")
	}
	for _, c := range erubiCorpus {
		src, magic, err := Compile(c.tpl, Options{Mode: ModeErubi})
		if err != nil {
			t.Fatalf("Compile(%q, erubi): %v", c.tpl, err)
		}
		if magic != "#coding:UTF-8\n" {
			t.Errorf("magic=%q for %q", magic, c.tpl)
		}
		ours := erubiEvalOurs(t, src, c.vars)
		want := erubiReference(t, c.tpl, c.vars)
		if ours != want {
			t.Errorf("RENDER mismatch tpl=%q\n ours=%q\n erubi=%q\n src=%q", c.tpl, ours, want, src)
		}
	}
}

// TestErubiDivergesFromClassicERB documents, against the live oracles, that
// ModeErubi is not any classic ERB trim_mode: it keeps the standalone
// expression newline that trim_mode "<>" trims, and auto-trims the standalone
// code line that trim_mode "" and "-" keep.
func TestErubiDivergesFromClassicERB(t *testing.T) {
	if !erubiAvailable(t) {
		t.Skip("ruby+erubi not available; skipping divergence test")
	}
	// "<%= n %>\n": erubi keeps the newline, classic "<>" trims it.
	exprTpl := "<%= n %>\n"
	vars := map[string]string{"n": "1"}
	erubiSrc, _, _ := Compile(exprTpl, Options{Mode: ModeErubi})
	trimSrc, _, _ := Compile(exprTpl, Options{TrimMode: "<>"})
	erubiOut := erubiEvalOurs(t, erubiSrc, vars)
	trimOut := mriEval(t, trimSrc, vars)
	if erubiOut != "1\n" {
		t.Errorf("erubi expr-line: got %q want %q", erubiOut, "1\n")
	}
	if trimOut != "1" {
		t.Errorf("classic <> expr-line: got %q want %q", trimOut, "1")
	}
	if erubiOut == trimOut {
		t.Errorf("expected erubi (%q) to differ from classic <> (%q)", erubiOut, trimOut)
	}

	// "<% x=1 %>\n": erubi trims the whole line, classic "" and "-" keep "\n".
	codeTpl := "<% x = 1 %>\n"
	erubiSrc2, _, _ := Compile(codeTpl, Options{Mode: ModeErubi})
	plainSrc, _, _ := Compile(codeTpl, Options{TrimMode: ""})
	if got := erubiEvalOurs(t, erubiSrc2, nil); got != "" {
		t.Errorf("erubi code-line: got %q want %q", got, "")
	}
	if got := mriEval(t, plainSrc, nil); got != "\n" {
		t.Errorf("classic '' code-line: got %q want %q", got, "\n")
	}
}

// erubiReference renders tpl through the real erubi gem's default Engine.
func erubiReference(t *testing.T, tpl string, vars map[string]string) string {
	t.Helper()
	script := `$stdout.binmode; require 'erubi'; ` + setupLocals(vars) +
		`tpl = $stdin.binmode.read.force_encoding("UTF-8"); ` +
		`print eval(Erubi::Engine.new(tpl).src)`
	return runRuby(t, script, tpl)
}

// erubiEvalOurs evals our ModeErubi-compiled source (which may reference
// ERB::Util.html_escape for "<%==") and returns the rendered buffer.
func erubiEvalOurs(t *testing.T, src string, vars map[string]string) string {
	t.Helper()
	script := `$stdout.binmode; require 'erb'; ` + setupLocals(vars) +
		`print eval($stdin.binmode.read.force_encoding("UTF-8"))`
	return runRuby(t, script, src)
}

func setupLocals(vars map[string]string) string {
	var b strings.Builder
	for k, v := range vars {
		b.WriteString(k + " = " + v + "; ")
	}
	return b.String()
}

func runRuby(t *testing.T, script, stdin string) string {
	t.Helper()
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby: %v\nscript=%s\nstdin=%q", err, script, stdin)
	}
	return string(out)
}
