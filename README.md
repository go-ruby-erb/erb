<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-erb/brand/main/social/go-ruby-erb-erb.png" alt="go-ruby-erb/erb" width="720"></p>

# erb — go-ruby-erb

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-erb.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's [ERB](https://docs.ruby-lang.org/en/master/ERB.html)
template compiler** — the deterministic, interpreter-independent core of MRI's
`ERB::Compiler`. It turns an ERB template string into the **Ruby source that
renders it**, matching MRI 4.0.5 byte-for-byte, and provides `ERB::Util`'s
`html_escape` / `url_encode` helpers.

It is the ERB backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime.

> **What it is — and isn't.** Compiling a template to Ruby source (tag scanning,
> trim modes, the `<%% %%>` literals, magic-comment detection, the `String#dump`
> text encoding) is fully deterministic and needs **no interpreter**, so it lives
> here as pure Go. The final `eval(compiled_src, binding)` that produces the
> rendered string **does** need a Ruby interpreter and stays in the consumer
> (e.g. rbgo) — this library compiles, the host evaluates.

## Features

Faithful port of MRI's `lib/erb/compiler.rb`, validated against the `ruby`
binary on every supported platform:

- **All tag kinds** — `<% code %>`, `<%= expression %>`, `<%# comment %>`.
- **Literals** — `<%%` → `<%` and the `%%>` escape, exactly as MRI scans them.
- **Every trim mode** — the MRI `trim_mode` string and its combinations:
  - `-` — `-%>` strips the immediately-following newline (explicit trim);
  - `>` — strips the newline after a tag that ends its line;
  - `<>` — strips the newline only when the tag both starts **and** ends the line;
  - `%` — `%`-prefixed lines are code lines, with `%%` an escaped literal `%`.
- **Magic comments** — a leading `<%# coding: … %>` / `frozen_string_literal: …`
  (including the emacs `-*- … -*-` form) is detected and reflected in the emitted
  `#coding:` / `#frozen-string-literal:` prefix.
- **Binary-exact text encoding** — literal runs are emitted via Ruby's
  `String#dump` on the binary string, so embedded quotes, newlines, control
  bytes and multi-byte UTF-8 round-trip byte-for-byte (`héllo` → `"h\xC3\xA9llo"`).
- **`ERB::Util`** — `HTMLEscape` (`&<>"'` → entities, `'` → `&#39;`) and
  `URLEncode` (percent-encode all but `A-Za-z0-9-_.~`, upper-case hex).
- **erubi dialect** (`Mode: ModeErubi`) — reproduces the [erubi](https://github.com/jeremyevans/erubi)
  gem's `Erubi::Engine` whitespace/trim semantics, so consumers that render through
  erubi (Sinatra, Rails) get byte-identical output. erubi matches **no** classic
  `trim_mode`: a standalone `<%= x %>\n` **keeps** its newline (whereas `<>` trims
  it), a line holding only a `<% … %>` code or `<%# … %>` comment tag is trimmed
  **automatically** (whereas the default keeps it and `-` needs explicit `<%- … -%>`),
  `-%>` / `=%>` chomps an expression's newline, and `<%==` HTML-escapes. Additive:
  the default `ModeERB` is unchanged, so `require "erb"` consumers are unaffected.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x).

## Install

```sh
go get github.com/go-ruby-erb/erb
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-erb/erb"
)

func main() {
	src, magic, err := erb.Compile("Hello <%= name %>!\n", erb.Options{})
	if err != nil {
		panic(err)
	}
	fmt.Print(magic) // #coding:UTF-8
	fmt.Println(src)
	// #coding:UTF-8
	// _erbout = +''; _erbout.<< "Hello ".freeze; _erbout.<<(( name ).to_s); _erbout.<< "!\n".freeze
	// ; _erbout
	//
	// src already carries the #coding prefix; hand it to a Ruby interpreter:
	//   eval(src, binding)  ->  "Hello World!\n"

	fmt.Println(erb.HTMLEscape(`<a href="x">it's</a>`))
	// &lt;a href=&quot;x&quot;&gt;it&#39;s&lt;/a&gt;
	fmt.Println(erb.URLEncode("100% & more"))
	// 100%25%20%26%20more
}
```

With a trim mode and a custom buffer variable:

```go
src, _, _ := erb.Compile("<ul>\n<% items.each do |i| -%>\n  <li><%= i %></li>\n<% end -%>\n</ul>\n",
	erb.Options{TrimMode: "-", EOutVar: "buf"})
```

In erubi-compatible mode (for Sinatra/Rails-style rendering) the standalone
`<%= … %>` newline is kept and standalone `<% … %>` lines are trimmed:

```go
// "<%= yield %>\n"  ->  renders "…\n" (newline kept), matching erubi — not "…"
src, _, _ := erb.Compile("<%= yield %>\n<% items.each do |i| %>\n<%= i %>\n<% end %>\n",
	erb.Options{Mode: erb.ModeErubi})
```

## API

```go
type Options struct {
	Mode     Mode   // ModeERB (default, classic MRI ERB) or ModeErubi (erubi-compatible)
	TrimMode string // MRI trim_mode: "", "-", ">", "<>", "%" and combinations (ModeERB only)
	EOutVar  string // output buffer var name; default "_erbout"
}

// Mode selects the ERB dialect Compile targets.
type Mode int
const (
	ModeERB   Mode = iota // classic MRI ERB (honours TrimMode)
	ModeErubi             // erubi gem's Erubi::Engine semantics (TrimMode ignored)
)

// Compile returns the Ruby source that, when eval'd against a binding, renders
// the template, plus the magic-encoding comment line — matching MRI's
// ERB::Compiler#compile two-value contract.
func Compile(template string, opts Options) (src, magicComment string, err error)

func HTMLEscape(s string) string // ERB::Util.html_escape / .h
func URLEncode(s string) string  // ERB::Util.url_encode / .u

// Compiler mirrors MRI's ERB::Compiler for hosts that need the put/insert/
// pre/post command wiring directly (most callers use Compile).
type Compiler struct { PutCmd, InsertCmd string; PreCmd, PostCmd []string; EOutVar string }
func NewCompiler(trimMode string) *Compiler
func (c *Compiler) Compile(s string) (src, magicComment string, err error)
```

## Tests & coverage

The suite includes a **differential oracle**: a wide template corpus (every tag
kind, the literals, all trim modes, multiline/quoted/unicode bodies) is compiled
both here and by the system `ruby`, comparing the emitted Ruby source **and** the
rendered result byte-for-byte, plus `ERB::Util` against MRI. A parallel oracle
renders the erubi-mode corpus against the installed **erubi gem** (`Erubi::Engine`)
for byte-identical parity.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

The oracle tests skip themselves where `ruby` is not on `PATH` (e.g. the qemu
arch lanes), so the cross-arch builds still validate the compiler itself.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-erb/erb authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
