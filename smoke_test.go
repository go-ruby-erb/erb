package erb

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func mriSrc(t *testing.T, tpl, mode string) string {
	t.Helper()
	// Read the (possibly NUL-bearing) template from stdin to avoid argv limits.
	script := `m = ARGV[0]; tpl = $stdin.binmode.read.force_encoding("UTF-8"); ` +
		`print ERB.new(tpl, trim_mode:(m=="" ? nil : m)).src`
	cmd := exec.Command("ruby", "-rerb", "-e", script, "--", mode)
	cmd.Stdin = strings.NewReader(tpl)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby: %v", err)
	}
	return string(out)
}

func TestSmokeSrcMatchesMRI(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("no ruby")
	}
	cases := []struct{ tpl, mode string }{
		{"abc", ""},
		{"Hello <%= name %>!\n<% x = 1 %>done\n", ""},
		{"x<%= a %>y", ""},
		{"a<%%b%%>c", ""},
		{"<%# c %>hi", ""},
		{"a\n<% x -%>\nb\n", "-"},
		{"a\n<% x %>\nb\n", ">"},
		{"a\n<% x %>\nb\n", "<>"},
		{"a<% x %>\nb", "<>"},
		{"%w = 3\nval=<%= w %>\n%% lit\n", "%"},
		{"héllo <%= 1 %> 😀", ""},
		{"<%# coding: us-ascii %>hi", ""},
	}
	for _, c := range cases {
		want := mriSrc(t, c.tpl, c.mode)
		got, _, err := Compile(c.tpl, Options{TrimMode: c.mode})
		if err != nil {
			t.Fatalf("Compile(%q,%q): %v", c.tpl, c.mode, err)
		}
		if got != want {
			t.Errorf("MISMATCH tpl=%q mode=%q\n got=%q\nwant=%q", c.tpl, c.mode, got, want)
		} else {
			fmt.Printf("OK tpl=%q mode=%q\n", c.tpl, c.mode)
		}
	}
}
