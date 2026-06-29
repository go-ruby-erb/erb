package erb

import (
	"os/exec"
	"strings"
	"testing"
)

// bigCorpus is a wide set of templates exercising every tag kind, the <%% / %%>
// literals, all trim modes, multiline bodies, embedded quotes/newlines/unicode,
// and tricky boundary cases. Each is compiled both here and by MRI and the
// emitted Ruby source compared byte-for-byte; a renderable subset is also
// eval'd by MRI to compare the final rendered string.
var corpusTemplates = []string{
	// Plain text.
	"",
	"plain text",
	"line1\nline2\nline3",
	"trailing newline\n",
	"tabs\tand\tquotes \" and ' and back\\slash",
	"unicode héllo wörld 日本語 😀 end",
	"control \x00\x01\x1f\x7f bytes",
	"a # b #{x} #@v #$g #[ literal hashes",
	// Code tags.
	"<% x = 1 %>",
	"a<% x = 1 %>b",
	"<% if true %>yes<% end %>",
	"<% [1,2].each do |i| %>i=<%= i %>\n<% end %>",
	// Expression tags.
	"<%= 1 + 2 %>",
	"x = <%= a %>, y = <%= b %>",
	"<%= \"quoted\" %>",
	"<%= name %>\n",
	// Comment tags.
	"<%# a comment %>after",
	"before<%# c %>after",
	"<%# multi\nline\ncomment %>x",
	// Literal escapes.
	"a<%%b",
	"a<%% literal open",
	"<%%= not an expr %%>",
	"a%%>b",
	"<%= 1 %><%%literal%%><% y=2 %>",
	// Multiline mixes.
	"top\n<% x=1 %>\nmid\n<%= x %>\nbot\n",
	"<%= 1 %>\n<%= 2 %>\n<%= 3 %>\n",
	// Newline-adjacent tags.
	"a\n<% c %>\nb",
	"<% c %>\n",
	"\n<% c %>",
	// Unterminated / edge.
	"text only no tags here",
	"just a < percent like <%notclosed",
	"%>orphan close",
}

// trimModes covers the empty mode and every combination ERB.new accepts.
var trimModes = []string{"", "-", ">", "<>", "%", "%-", "%>", "%<>", "-%", ">%"}

func TestDifferentialSrcAgainstMRI(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; skipping differential test")
	}
	for _, mode := range trimModes {
		for _, tpl := range corpusTemplates {
			want := mriSrc(t, tpl, mode)
			got, _, err := Compile(tpl, Options{TrimMode: mode})
			if err != nil {
				t.Fatalf("Compile(%q, mode=%q): %v", tpl, mode, err)
			}
			if got != want {
				t.Errorf("SRC mismatch tpl=%q mode=%q\n got=%q\nwant=%q", tpl, mode, got, want)
			}
		}
	}
}

// renderTemplates are templates whose compiled source is safely evaluable with
// only the locals we provide, used for end-to-end rendered-output comparison.
var renderTemplates = []struct {
	tpl  string
	vars map[string]string // local name -> ruby literal
}{
	{"Hello <%= name %>!", map[string]string{"name": `"World"`}},
	{"<% 3.times do |i| %><%= i %><% end %>", nil},
	{"sum=<%= a + b %>", map[string]string{"a": "2", "b": "5"}},
	{"a<%%b%%>c<%= x %>", map[string]string{"x": `"!"`}},
	{"<%# hidden %>shown <%= y %>", map[string]string{"y": "42"}},
	{"top\n<% if flag %>on<% else %>off<% end %>\nbot", map[string]string{"flag": "true"}},
	{"list:\n<% items.each do |it| %>- <%= it %>\n<% end %>", map[string]string{"items": `["x","y","z"]`}},
	{"héllo <%= who %> 😀", map[string]string{"who": `"monde"`}},
	{"quote <%= q %>", map[string]string{"q": `"a\"b'c"`}},
}

func TestDifferentialRenderAgainstMRI(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; skipping differential test")
	}
	for _, mode := range []string{"", "-", ">", "<>", "%"} {
		for _, rc := range renderTemplates {
			got, _, err := Compile(rc.tpl, Options{TrimMode: mode})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			want := mriRender(t, rc.tpl, mode, rc.vars)
			// Render OUR compiled source via ruby eval to compare rendering.
			oursRendered := mriEval(t, got, rc.vars)
			if oursRendered != want {
				t.Errorf("RENDER mismatch tpl=%q mode=%q\n ours=%q\n mri =%q", rc.tpl, mode, oursRendered, want)
			}
		}
	}
}

// mriRender returns MRI's ERB.new(tpl, trim_mode:).result with the given locals.
func mriRender(t *testing.T, tpl, mode string, vars map[string]string) string {
	t.Helper()
	var setup strings.Builder
	for k, v := range vars {
		setup.WriteString(k + " = " + v + "; ")
	}
	script := setup.String() +
		`m = ARGV[0]; tpl = $stdin.binmode.read.force_encoding("UTF-8"); ` +
		`print ERB.new(tpl, trim_mode:(m=="" ? nil : m)).result(binding)`
	cmd := exec.Command("ruby", "-rerb", "-e", script, "--", mode)
	cmd.Stdin = strings.NewReader(tpl)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby render %q: %v", tpl, err)
	}
	return string(out)
}

// mriEval evals our already-compiled source under ruby with the given locals,
// returning the rendered buffer. This proves our emitted source renders
// identically to MRI's, not merely that the source strings match.
func mriEval(t *testing.T, src string, vars map[string]string) string {
	t.Helper()
	var setup strings.Builder
	for k, v := range vars {
		setup.WriteString(k + " = " + v + "; ")
	}
	// The compiled src (read from stdin) ends with "; _erbout"; eval returns it.
	script := setup.String() + `print eval($stdin.binmode.read.force_encoding("UTF-8"))`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby eval of our src: %v\nsrc=%q", err, src)
	}
	return string(out)
}
