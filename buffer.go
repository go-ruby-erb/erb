package erb

import "strings"

// buffer accumulates the compiled Ruby source, mirroring MRI's Compiler::Buffer.
// Commands are collected into a line and flushed (joined by "; ") on cr/close.
type buffer struct {
	line   []string
	script strings.Builder
}

// newBuffer builds a buffer seeded with the magic comment(s) and the compiler's
// pre-commands, mirroring Buffer#initialize. enc is always non-empty here (we
// default it to "UTF-8"); frozen is the optional frozen_string_literal value or
// "" when absent.
func newBuffer(c *Compiler, enc, frozen string) *buffer {
	b := &buffer{}
	if enc != "" {
		b.script.WriteString("#coding:" + enc + "\n")
	}
	if frozen != "" {
		b.script.WriteString("#frozen-string-literal:" + frozen + "\n")
	}
	for _, x := range c.PreCmd {
		b.push(x)
	}
	return b
}

// push appends a command to the current line (MRI Buffer#push).
func (b *buffer) push(cmd string) {
	b.line = append(b.line, cmd)
}

// cr flushes the current line, joined by "; ", and starts a new one with a
// trailing newline (MRI Buffer#cr).
func (b *buffer) cr() {
	b.script.WriteString(strings.Join(b.line, "; "))
	b.line = b.line[:0]
	b.script.WriteByte('\n')
}

// close flushes the final line after appending the post-commands (MRI
// Buffer#close).
func (b *buffer) close(c *Compiler) {
	for _, x := range c.PostCmd {
		b.push(x)
	}
	b.script.WriteString(strings.Join(b.line, "; "))
	b.line = nil
}

// addPutCmd emits a literal-text append: `<put_cmd> "<dumped>".freeze` followed
// by one newline per newline in the content (MRI Compiler#add_put_cmd). The
// trailing newlines keep the compiled source's line count aligned with the
// template's, so eval-time line numbers match.
func (b *buffer) addPutCmd(c *Compiler, content string) {
	b.push(c.PutCmd + " " + rubyDump(content) + ".freeze" + strings.Repeat("\n", strings.Count(content, "\n")))
}

// addInsertCmd emits an expression append: `<insert_cmd>((<content>).to_s)`
// (MRI Compiler#add_insert_cmd).
func (b *buffer) addInsertCmd(c *Compiler, content string) {
	b.push(c.InsertCmd + "((" + content + ").to_s)")
}
