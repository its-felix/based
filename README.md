# based

[![Go Reference](https://pkg.go.dev/badge/github.com/its-felix/based.svg)](https://pkg.go.dev/github.com/its-felix/based)
[![Go Report Card](https://goreportcard.com/badge/github.com/its-felix/based)](https://goreportcard.com/report/github.com/its-felix/based)
[![Test](https://github.com/its-felix/based/actions/workflows/test.yml/badge.svg)](https://github.com/its-felix/based/actions/workflows/test.yml)
[![License](https://img.shields.io/github/license/its-felix/based)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/its-felix/based)](go.mod)

`based` is a generic base-N encoder for Go byte streams. It turns `[]byte`
into tokens from a caller-provided dictionary, and decodes those tokens back to
the original bytes.

The standard library already has excellent, highly optimized encoders for
common base32 and base64 formats. `based` is for the cases where the alphabet is
the important part: custom alphabets, application-specific token sets, or output
types that are not bytes at all.

## Goals

- Encode arbitrary byte streams with any unique dictionary of two or more words.
- Support byte-oriented alphabets as well as non-byte word types such as
  strings, integers, or small domain-specific token structs.
- Provide familiar append-style and streaming APIs for low-allocation use.
- Include predefined encodings for standard base32/base64 variants when a
  generic implementation is convenient.

## Use Cases

- Short IDs with a restricted alphabet, for example Crockford-style base32 or
  URL-safe tokens.
- Human-facing codes that avoid ambiguous characters such as `0`, `O`, `I`, and
  `l`.
- Encoding bytes into application tokens, syllables, dictionary words, emoji, or
  numeric symbols.
- Protocols or storage formats that need deterministic reversible tokenization
  over a custom alphabet.
- Tests, tools, and experiments where you want one encoder implementation across
  many dictionary sizes.

If you only need standard base64 or base32 throughput, prefer Go's
`encoding/base64` or `encoding/base32`. They are much faster for those exact
formats. `based` trades that specialized speed for generic dictionaries and
generic word types.

## Installation

```sh
go get github.com/its-felix/based
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/its-felix/based"
)

func main() {
	raw := []byte("hello world")

	encoded := based.Base64RawURLEncoding.AppendEncode(nil, raw)
	fmt.Printf("%s\n", encoded)

	decoded, err := based.Base64RawURLEncoding.AppendDecode(nil, encoded)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", decoded)
}
```

## Custom Byte Alphabet

Create an encoding from any unique byte alphabet. Padding is optional for small
alphabets and can be supplied as a second argument.

```go
package main

import (
	"bytes"
	"fmt"

	"github.com/its-felix/based"
)

func main() {
	alphabet := []byte("0123456789ABCDEFGHJKLMNPQRSTUVWXYZ")
	enc := based.MustNewEncoding(alphabet)

	raw := []byte{0xde, 0xad, 0xbe, 0xef}
	token := enc.AppendEncode(nil, raw)

	roundTrip, err := enc.AppendDecode(nil, token)
	if err != nil {
		panic(err)
	}

	fmt.Printf("token: %s\n", token)
	fmt.Println(bytes.Equal(raw, roundTrip))
}
```

## Non-Byte Tokens

The dictionary word type only needs to be comparable. This lets the encoded
output be `[]string`, `[]uint`, or another comparable token type.

```go
package main

import (
	"fmt"

	"github.com/its-felix/based"
)

func main() {
	type Word string

	dict := []Word{"ba", "be", "bi", "bo", "bu", "da", "de", "di"}
	enc := based.MustNewEncoding(dict)

	encoded := enc.AppendEncode(nil, []byte("go"))
	fmt.Println(encoded)

	decoded, err := enc.AppendDecode(nil, encoded)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", decoded)
}
```

## Streaming

Use `NewWriter` to encode bytes as they are written and `NewReader` to decode
tokens as they are read.

```go
package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/its-felix/based"
)

func main() {
	var encoded bytes.Buffer

	w := based.NewWriter(based.Base64RawEncoding, &encoded)
	if _, err := w.Write([]byte("stream me")); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}

	r := based.NewReader(based.Base64RawEncoding, &encoded)
	decoded, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", decoded)
}
```

## Predefined Encodings

- `Base32RawEncoding`
- `Base32RawHexEncoding`
- `Base64Encoding`
- `Base64URLEncoding`
- `Base64RawEncoding`
- `Base64RawURLEncoding`

## API Notes

- `NewEncoding` validates the dictionary and returns an error.
- `MustNewEncoding` panics on invalid dictionaries and is useful for package
  globals.
- Dictionary words must be unique.
- A padding word, when provided, must not be part of the dictionary.
- Encodings with dictionaries of 512 or more words require padding.
- `AppendEncode` and `AppendDecode` follow the standard Go append-style pattern.
- `EncodedLen` and `DecodedLen` are available when preallocating buffers.

## Benchmarks

The benchmark suite compares `based` with Go's standard `encoding/base32` and
`encoding/base64` append APIs. Each case encodes or decodes a 32 KiB payload,
reuses the destination buffer, and reports throughput over the original payload
size.

Run the benchmarks with:

```sh
go test -run '^$' -bench 'BenchmarkAppend' -benchmem -count 5 ./...
```

The results below are the median of 5 runs on:

```text
goos: darwin
goarch: arm64
cpu: Apple M1 Pro
go: go1.26.1
```

| Operation | Encoding | stdlib | based | based vs stdlib | based allocs |
| --- | --- | ---: | ---: | ---: | ---: |
| Encode | base64 padded | 1849 MB/s | 160 MB/s | 0.09x | 0 B/op, 0 allocs/op |
| Encode | base64 raw | 1860 MB/s | 159 MB/s | 0.09x | 0 B/op, 0 allocs/op |
| Encode | base32 padded | 1791 MB/s | 105 MB/s | 0.06x | 0 B/op, 0 allocs/op |
| Encode | base32 raw | 1800 MB/s | 130 MB/s | 0.07x | 0 B/op, 0 allocs/op |
| Decode | base64 padded | 2252 MB/s | 62 MB/s | 0.03x | 21 B/op, 0 allocs/op |
| Decode | base64 raw | 2249 MB/s | 67 MB/s | 0.03x | 0 B/op, 0 allocs/op |
| Decode | base32 padded | 287 MB/s | 57 MB/s | 0.20x | 23 B/op, 0 allocs/op |
| Decode | base32 raw | 288 MB/s | 59 MB/s | 0.20x | 0 B/op, 0 allocs/op |

Throughput sketch, in MB/s:

```text
Encode base64 padded  stdlib ################### 1849 | based ## 160
Encode base64 raw     stdlib ################### 1860 | based ## 159
Encode base32 padded  stdlib ##################  1791 | based #  105
Encode base32 raw     stdlib ##################  1800 | based #  130
Decode base64 padded  stdlib ###################### 2252 | based #  62
Decode base64 raw     stdlib ###################### 2249 | based #  67
Decode base32 padded  stdlib ### 287 | based # 57
Decode base32 raw     stdlib ### 288 | based # 59
```

The standard library is much faster for the built-in base32/base64 alphabets.
`based` trades that specialized performance for a generic implementation that
can use custom dictionaries and non-byte word types.
