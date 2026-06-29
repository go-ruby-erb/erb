package erb

import (
	"os/exec"
	"strings"
	"testing"
)

// TestCompileGolden pins a representative set of (template, mode) -> emitted
// Ruby source so the contract is locked even where ruby is unavailable (CI
// arch/qemu lanes). The golden strings were captured from MRI 4.0.5.
func TestCompileGolden(t *testing.T) {
	cases := []struct {
		name, tpl, mode, want string
	}{
		{
			"plain", "abc", "",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"abc\".freeze; _erbout",
		},
		{
			"expr", "<%= x %>", "",
			"#coding:UTF-8\n_erbout = +''; _erbout.<<(( x ).to_s); _erbout",
		},
		{
			"code", "<% y %>", "",
			"#coding:UTF-8\n_erbout = +'';  y ; _erbout",
		},
		{
			"comment", "<%# c %>", "",
			"#coding:UTF-8\n_erbout = +''; ; _erbout",
		},
		{
			"literal-open", "a<%%b", "",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a<%b\".freeze; _erbout",
		},
		{
			"literal-close-in-text", "a%%>b", "",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a%%>b\".freeze; _erbout",
		},
		{
			"multiline-text", "li1\nli2", "",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"li1\\nli2\".freeze\n; _erbout",
		},
		{
			"dash", "a\n<% x -%>\nb\n", "-",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\\n\".freeze\n;  x \n_erbout.<< \"b\\n\".freeze\n; _erbout",
		},
		{
			"gt", "a\n<% x %>\nb\n", ">",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\\n\".freeze\n;  x \n_erbout.<< \"b\\n\".freeze\n; _erbout",
		},
		{
			"lgt", "a\n<% x %>\nb\n", "<>",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\\n\".freeze\n;  x \n_erbout.<< \"b\\n\".freeze\n; _erbout",
		},
		{
			"lgt-inline", "a<% x %>\nb", "<>",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze;  x ; _erbout.<< \"\\n\".freeze\n; _erbout.<< \"b\".freeze; _erbout",
		},
		{
			"percent", "%w = 3\nval=<%= w %>\n%% lit\n", "%",
			"#coding:UTF-8\n_erbout = +''; w = 3\n_erbout.<< \"val=\".freeze; _erbout.<<(( w ).to_s); _erbout.<< \"\\n\".freeze\n; _erbout.<< \"% lit\\n\".freeze\n; _erbout",
		},
		{
			"gt-crlf", "a\r\n<% x %>\r\nb", ">",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\\r\\n\".freeze\n;  x \n_erbout.<< \"b\".freeze; _erbout",
		},
		{
			"dash-midline-noNL", "a<% x -%>b", "-",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze;  x ; _erbout.<< \"b\".freeze; _erbout",
		},
		{
			"dash-indented-open", "  <%- x %>y", "-",
			"#coding:UTF-8\n_erbout = +'';  x ; _erbout.<< \"y\".freeze; _erbout",
		},
		{
			"dash-inline-open", "a<%- x %>b", "-",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze;  x ; _erbout.<< \"b\".freeze; _erbout",
		},
		{
			"dash-literal-close-in-tag", "a<%= 1 %%> %>b", "-",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze; _erbout.<<(( 1 %> ).to_s); _erbout.<< \"b\".freeze; _erbout",
		},
		{
			"comment-multiline-lineno", "<%# l1\nl2\nl3 %>after", "",
			"#coding:UTF-8\n_erbout = +''; \n\n; _erbout.<< \"after\".freeze; _erbout",
		},
		{
			"code-body-newline", "<% x\n%>y", "",
			"#coding:UTF-8\n_erbout = +'';  x\n_erbout.<< \"y\".freeze; _erbout",
		},
		{
			"lgt-tag-mid-line", "a<% x %> mid\nb", "<>",
			"#coding:UTF-8\n_erbout = +''; _erbout.<< \"a\".freeze;  x ; _erbout.<< \" mid\\n\".freeze\n; _erbout.<< \"b\".freeze; _erbout",
		},
		{
			"percent-frozen-directive", "%#frozen_string_literal: true\nhi", "%",
			"#coding:UTF-8\n#frozen-string-literal:true\n_erbout = +''; #frozen_string_literal: true\n_erbout.<< \"hi\".freeze; _erbout",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := Compile(c.tpl, Options{TrimMode: c.mode})
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if got != c.want {
				t.Errorf("\n got=%q\nwant=%q", got, c.want)
			}
		})
	}
}

func TestCompileMagicComment(t *testing.T) {
	src, magic, err := Compile("abc", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if magic != "#coding:UTF-8\n" {
		t.Errorf("magic = %q", magic)
	}
	if !strings.HasPrefix(src, "#coding:UTF-8\n") {
		t.Errorf("src prefix = %q", src)
	}
}

func TestCompileEOutVar(t *testing.T) {
	got, _, err := Compile("<%= x %>", Options{EOutVar: "buf"})
	if err != nil {
		t.Fatal(err)
	}
	want := "#coding:UTF-8\nbuf = +''; buf.<<(( x ).to_s); buf"
	if got != want {
		t.Errorf("\n got=%q\nwant=%q", got, want)
	}
}

func TestCompileCodingDirective(t *testing.T) {
	got, magic, err := Compile("<%# -*- coding: us-ascii -*- %>hi", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if magic != "#coding:US-ASCII\n" {
		t.Errorf("magic = %q", magic)
	}
	if !strings.HasPrefix(got, "#coding:US-ASCII\n") {
		t.Errorf("src = %q", got)
	}
}

func TestCompileFrozenDirective(t *testing.T) {
	got, _, err := Compile("<%# frozen_string_literal: true %>hi", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "#frozen-string-literal:true\n") {
		t.Errorf("missing frozen comment: %q", got)
	}
}

func TestCompilePercentCodingDirective(t *testing.T) {
	got, magic, err := Compile("%#coding:us-ascii\nhi", Options{TrimMode: "%"})
	if err != nil {
		t.Fatal(err)
	}
	if magic != "#coding:US-ASCII\n" {
		t.Errorf("magic = %q", magic)
	}
	if !strings.HasPrefix(got, "#coding:US-ASCII\n") {
		t.Errorf("src = %q", got)
	}
}

func TestNewCompilerDefaults(t *testing.T) {
	c := NewCompiler("")
	if c.PutCmd != "print" || c.InsertCmd != "print" {
		t.Errorf("defaults = %q/%q, want print/print", c.PutCmd, c.InsertCmd)
	}
	if c.PreCmd != nil || c.PostCmd != nil {
		t.Errorf("pre/post should be nil by default")
	}
	// A print-based compile (MRI's default Compiler wiring) emits print(...).
	got, _, err := c.Compile("Got <%= obj %>!\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "#coding:UTF-8\nprint \"Got \".freeze; print(( obj ).to_s); print \"!\\n\".freeze\n"
	if got != want {
		t.Errorf("\n got=%q\nwant=%q", got, want)
	}
}

func TestPrepareTrimMode(t *testing.T) {
	cases := []struct {
		in       string
		wantPerc bool
		wantTrim string
	}{
		{"", false, ""},
		{"-", false, "-"},
		{">", false, ">"},
		{"<>", false, "<>"},
		{"%", true, ""},
		{"%-", true, "-"},
		{"%>", true, ">"},
		{"%<>", true, "<>"},
		{"->", false, "-"},  // "-" wins over ">"
		{"<>-", false, "-"}, // "-" wins over "<>"
	}
	for _, c := range cases {
		p, m := prepareTrimMode(c.in)
		if p != c.wantPerc || m != c.wantTrim {
			t.Errorf("prepareTrimMode(%q) = (%v,%q), want (%v,%q)", c.in, p, m, c.wantPerc, c.wantTrim)
		}
	}
}

func TestHTMLEscape(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"plain":        "plain",
		"a&b":          "a&amp;b",
		"<x>":          "&lt;x&gt;",
		"\"q\"":        "&quot;q&quot;",
		"it's":         "it&#39;s",
		"a&b<c>d\"e'f": "a&amp;b&lt;c&gt;d&quot;e&#39;f",
		"café & naïve": "café &amp; naïve",
	}
	for in, want := range cases {
		if got := HTMLEscape(in); got != want {
			t.Errorf("HTMLEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestURLEncode(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"plain":       "plain",
		"~-_.":        "~-_.",
		"a b":         "a%20b",
		"a&b":         "a%26b",
		"100% & more": "100%25%20%26%20more",
		"a/b?c=d#e":   "a%2Fb%3Fc%3Dd%23e",
		"café":        "caf%C3%A9",
		"😀":           "%F0%9F%98%80",
	}
	for in, want := range cases {
		if got := URLEncode(in); got != want {
			t.Errorf("URLEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRubyDump(t *testing.T) {
	cases := map[string]string{
		"abc":          `"abc"`,
		"a\"b":         `"a\"b"`,
		"a\\b":         `"a\\b"`,
		"a\nb":         `"a\nb"`,
		"\a\b\t\v\f\r": `"\a\b\t\v\f\r"`,
		"\x1b":         `"\e"`,
		"a#b":          `"a#b"`,
		"a#{x}":        `"a\#{x}"`,
		"a#@v":         `"a\#@v"`,
		"a#$g":         `"a\#$g"`,
		"a#[":          `"a#["`,
		"\x00\x7f":     `"\x00\x7F"`,
		"héllo":        `"h\xC3\xA9llo"`,
	}
	for in, want := range cases {
		if got := rubyDump(in); got != want {
			t.Errorf("rubyDump(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalEncoding(t *testing.T) {
	cases := map[string]string{
		"utf-8":          "UTF-8",
		"US-ASCII":       "US-ASCII",
		"ascii":          "US-ASCII",
		"shift_jis":      "Shift_JIS",
		"utf-8-unix":     "UTF-8", // EOL marker stripped
		"utf-8-mac":      "UTF-8",
		"utf-8-dos":      "UTF-8",
		"binary":         "ASCII-8BIT",
		"some-unknown-x": "SOME-UNKNOWN-X", // passthrough upcased
	}
	for in, want := range cases {
		if got := canonicalEncoding(in); got != want {
			t.Errorf("canonicalEncoding(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripEmacsWrapper(t *testing.T) {
	cases := map[string]string{
		" -*- coding: utf-8 -*- ": "coding: utf-8",
		" no wrapper here ":       " no wrapper here ",
		"-*-":                     "-*-", // no inner closing distinct from open
	}
	for in, want := range cases {
		if got := stripEmacsWrapper(in); got != want {
			t.Errorf("stripEmacsWrapper(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPercentMagicCommentNoNewline(t *testing.T) {
	// A "%#coding:..." magic line with no trailing newline (end of source): the
	// magic-comment scanner must still leave the default encoding (it cannot
	// match a percent comment without its terminating newline).
	_, magic, err := Compile("%#coding:us-ascii", Options{TrimMode: "%"})
	if err != nil {
		t.Fatal(err)
	}
	if magic != "#coding:UTF-8\n" {
		t.Errorf("magic = %q, want default (unterminated %% comment ignored)", magic)
	}
}

func TestLgtBlankLine(t *testing.T) {
	// A bare blank line in "<>" mode produces a "\n" text token, exercising the
	// head reset. Compared against MRI.
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH")
	}
	tpl := "a\n\n<% x %>\nb"
	got, _, err := Compile(tpl, Options{TrimMode: "<>"})
	if err != nil {
		t.Fatal(err)
	}
	if got != mriSrc(t, tpl, "<>") {
		t.Errorf("lgt blank-line mismatch: %q", got)
	}
}

func TestChomp(t *testing.T) {
	cases := map[string]string{
		"abc":     "abc",
		"abc\n":   "abc",
		"abc\r\n": "abc",
		"abc\r":   "abc\r", // a bare \r is not stripped by default chomp
		"":        "",
	}
	for in, want := range cases {
		if got := chomp(in); got != want {
			t.Errorf("chomp(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDashCRLF(t *testing.T) {
	// -%> followed by CRLF strips the CRLF (the :cr path of the explicit
	// scanner), matching MRI.
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH")
	}
	for _, tpl := range []string{"a\r\n<% x -%>\r\nb", "a\n<% x -%>\nb"} {
		got, _, err := Compile(tpl, Options{TrimMode: "-"})
		if err != nil {
			t.Fatal(err)
		}
		if got != mriSrc(t, tpl, "-") {
			t.Errorf("dash CRLF mismatch for %q: %q", tpl, got)
		}
	}
}

func TestScanDirectiveMixedCase(t *testing.T) {
	// A directive whose key has uppercase letters is matched case-insensitively
	// (normalizeKey lower-cases), exercising the upper-case branch.
	if v, ok := scanDirective("CODING: us-ascii", "coding"); !ok || v != "us-ascii" {
		t.Errorf("scanDirective mixed-case = (%q,%v)", v, ok)
	}
}

func TestScanDirectiveNoColon(t *testing.T) {
	// "coding" present but no ":"/"=" => not a directive.
	if v, ok := scanDirective(" coding without colon ", "coding"); ok {
		t.Errorf("expected no match, got %q", v)
	}
	// key present but value empty after colon => no match.
	if v, ok := scanDirective(" coding: ", "coding"); ok {
		t.Errorf("expected no value, got %q", v)
	}
	// key absent.
	if _, ok := scanDirective("nothing here", "coding"); ok {
		t.Error("expected no match for absent key")
	}
}

func TestUnterminatedConstructs(t *testing.T) {
	// These mirror MRI: an unterminated tag/comment swallows the rest as content
	// of the open tag; magic-comment scan with no closer leaves defaults.
	for _, tpl := range []string{"<% unterminated", "<%# unterminated comment", "<%= no close"} {
		if _, _, err := Compile(tpl, Options{}); err != nil {
			t.Errorf("Compile(%q) err = %v", tpl, err)
		}
	}
}

// TestUtilDifferentialAgainstMRI compares HTMLEscape/URLEncode to ERB::Util.
func TestUtilDifferentialAgainstMRI(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH")
	}
	inputs := []string{
		"a&b<c>d\"e'f", "plain", "", "100% & more", "<script>",
		"café & naïve", "a b/c?d=e#f", "~-_.", "😀ünïcödé", "tab\tnl\n",
	}
	for _, in := range inputs {
		gotH := HTMLEscape(in)
		gotU := URLEncode(in)
		wantH := mriUtil(t, "html_escape", in)
		wantU := mriUtil(t, "url_encode", in)
		if gotH != wantH {
			t.Errorf("HTMLEscape(%q) = %q, MRI %q", in, gotH, wantH)
		}
		if gotU != wantU {
			t.Errorf("URLEncode(%q) = %q, MRI %q", in, gotU, wantU)
		}
	}
}

func mriUtil(t *testing.T, fn, in string) string {
	t.Helper()
	script := `$stdout.binmode; s = $stdin.binmode.read.force_encoding("UTF-8"); print ERB::Util.` + fn + `(s)`
	cmd := exec.Command("ruby", "-rerb", "-e", script)
	cmd.Stdin = strings.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby util %s(%q): %v", fn, in, err)
	}
	return string(out)
}
