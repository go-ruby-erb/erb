package erb

import (
	"strings"
	"testing"
)

// TestCompileGoldenErubi pins (template -> emitted Ruby source) for the erubi
// dialect so the contract — and full coverage of the erubi compiler — holds even
// where ruby/erubi is unavailable (the Windows and qemu CI lanes). Each golden
// string's rendered result is independently proven byte-identical to the real
// erubi gem by TestErubiRenderMatchesErubi. The cases together exercise every
// branch of compileErubi: each tag kind, both escape indicators, the standalone
// vs non-standalone trim paths, every lspace-capture case, and the "-%>" chomp.
func TestCompileGoldenErubi(t *testing.T) {
	cases := []struct {
		name, tpl, want string
	}{
		{"empty", "", "#coding:UTF-8\n_erbout = +''; _erbout"},
		{"plain", "plain", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"plain\".freeze; _erbout"},
		{"expr-keeps-nl", "<%= x %>\n", "#coding:UTF-8\n_erbout = +''; _erbout.<<(( x ).to_s); _erbout.<< \"\\n\".freeze; _erbout"},
		{"expr-chomp", "<%= x -%>\n", "#coding:UTF-8\n_erbout = +''; _erbout.<<(( x ).to_s); _erbout"},
		{"expr-escaped", "<%== v %>", "#coding:UTF-8\n_erbout = +''; _erbout.<< ERB::Util.html_escape(( v )); _erbout"},
		{"code-standalone-bol", "<% x %>\n", "#coding:UTF-8\n_erbout = +'';  x \n; _erbout"},
		{"code-standalone-indent", "  <% x %>\n", "#coding:UTF-8\n_erbout = +'';    x \n; _erbout"},
		{"code-adjacent-notbol", "<% x %><% y %>\n", "#coding:UTF-8\n_erbout = +'';  x ;  y ; _erbout.<< \"\\n\".freeze; _erbout"},
		{"code-after-newline", "a\n<% x %>\n", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\\n\".freeze;  x \n; _erbout"},
		{"code-after-nl-indent", "a\n  <% x %>\n", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\\n\".freeze;    x \n; _erbout"},
		{"code-inline-notws", "x<% y %>z", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"x\".freeze;  y ; _erbout.<< \"z\".freeze; _erbout"},
		{"code-indent-norspace", "  <% x %>tail", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"  \".freeze;  x ; _erbout.<< \"tail\".freeze; _erbout"},
		{"code-notrim", "a<% x %>b", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze;  x ; _erbout.<< \"b\".freeze; _erbout"},
		{"comment-standalone", "<%# c %>\n", "#coding:UTF-8\n_erbout = +''; \n; _erbout"},
		{"comment-indent-norspace", "  <%# c %>x\n", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"  \".freeze; ; _erbout.<< \"x\\n\".freeze; _erbout"},
		{"comment-notbol-rspace", "x<%# c %>\n", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"x\".freeze; \n; _erbout.<< \"\\n\".freeze; _erbout"},
		{"literal-tailch", "pre<%% a -%>post\n", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"pre\".freeze; _erbout.<< \"<% a -%>\".freeze; _erbout.<< \"post\\n\".freeze; _erbout"},
		{"literal-plain", "a<%% b %%>c", "#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze; _erbout.<< \"<% b %%>\".freeze; _erbout.<< \"c\".freeze; _erbout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, magic, err := Compile(c.tpl, Options{Mode: ModeErubi})
			if err != nil {
				t.Fatalf("Compile(%q, erubi): %v", c.tpl, err)
			}
			if magic != "#coding:UTF-8\n" {
				t.Errorf("magic = %q, want %q", magic, "#coding:UTF-8\n")
			}
			if got != c.want {
				t.Errorf("src mismatch for %q\n got=%q\nwant=%q", c.tpl, got, c.want)
			}
		})
	}
}

// TestErubiModeIgnoresTrimMode confirms the additive contract: in ModeErubi the
// classic TrimMode field has no effect (erubi has no trim_mode), and a custom
// EOutVar is still honoured.
func TestErubiModeIgnoresTrimMode(t *testing.T) {
	base, _, _ := Compile("<%= x %>\n", Options{Mode: ModeErubi})
	for _, m := range []string{"", "-", ">", "<>", "%"} {
		got, _, _ := Compile("<%= x %>\n", Options{Mode: ModeErubi, TrimMode: m})
		if got != base {
			t.Errorf("TrimMode %q changed erubi output:\n got=%q\nbase=%q", m, got, base)
		}
	}
	got, _, _ := Compile("<%= x %>\n", Options{Mode: ModeErubi, EOutVar: "buf"})
	if !strings.HasPrefix(got, "#coding:UTF-8\nbuf = +''; buf.<<(( x ).to_s)") {
		t.Errorf("EOutVar not honoured in erubi mode: %q", got)
	}
}

// TestErubiSrcKeepsExpressionNewline is a ruby-free regression guard for the
// headline divergence: the erubi dialect bakes the kept expression-line newline
// into the emitted source, while classic "<>" trims it.
func TestErubiSrcKeepsExpressionNewline(t *testing.T) {
	erubiSrc, _, err := Compile("<%= n %>\n", Options{Mode: ModeErubi})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(erubiSrc, `.<< "\n".freeze`) {
		t.Errorf("erubi src should append the trailing newline as text; src=%q", erubiSrc)
	}
	trimSrc, _, _ := Compile("<%= n %>\n", Options{TrimMode: "<>"})
	if strings.Contains(trimSrc, `"\n"`) {
		t.Errorf("classic <> src should have trimmed the newline; src=%q", trimSrc)
	}
}
