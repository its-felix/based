package based

import (
	"bytes"
	"embed"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"slices"
	"testing"
)

//go:embed testdata/*.bin
var fs embed.FS

type testInput[T any] struct {
	raw      []byte
	expected []T
}

type buffer[T any] struct {
	buf []T
	off int
}

func (b *buffer[T]) grow(n int) int {
	m := len(b.buf) - b.off

	c := len(b.buf) + b.off + n // ensure enough space for n elements
	b2 := append([]T(nil), make([]T, c)...)
	i := copy(b2, b.buf)
	b.buf = b2[:i]

	b.off = 0
	b.buf = b.buf[:m+n]
	return m
}

func (b *buffer[T]) Write(p []T) (n int, err error) {
	m := b.grow(len(p))
	return copy(b.buf[m:], p), nil
}

func (b *buffer[T]) Read(p []T) (n int, err error) {
	if b.off >= len(b.buf) {
		return 0, io.EOF
	}

	n = copy(p, b.buf[b.off:])
	b.off += n

	return n, nil
}

func (b *buffer[T]) Values() []T {
	return b.buf[b.off:]
}

type limitedReader[T any] struct {
	data []T
	off  int
	max  int
}

func (r *limitedReader[T]) Read(p []T) (n int, err error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	if len(p) > r.max {
		p = p[:r.max]
	}

	n = copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

type encoder interface {
	AppendEncode(dst, src []byte) []byte
}

func TestComparisons(t *testing.T) {
	runComparisonTests(t, "base64std", base64.StdEncoding, Base64Encoding)
	runComparisonTests(t, "base64url", base64.URLEncoding, Base64URLEncoding)
	runComparisonTests(t, "base32rawstd", base32.StdEncoding.WithPadding(base32.NoPadding), Base32RawEncoding)
	runComparisonTests(t, "base32rawhex", base32.HexEncoding.WithPadding(base32.NoPadding), Base32RawHexEncoding)
	runComparisonTests(t, "base64rawstd", base64.RawStdEncoding, Base64RawEncoding)
	runComparisonTests(t, "base64rawurl", base64.RawURLEncoding, Base64RawURLEncoding)
}

func TestGeneric_Predefined(t *testing.T) {
	for i := 2; i <= 255; i++ {
		t.Run(fmt.Sprintf("generic_%d", i), func(t *testing.T) {
			expected, err := fs.ReadFile(fmt.Sprintf("testdata/%d.bin", i))
			if err != nil {
				t.Fatal(err)
				return
			}

			enc := newEncoder(i, func(i int) byte {
				return byte(i)
			})

			r := newTestRand()
			// files with expected output were generated with exactly these sizes and generator
			// if sizes are changed in any way the input files have to be re-generated too
			sizes := []int{1, 8, 12, 39, 78, 182, 304, 790, 3000, 5029, 13020, 90389}
			tests := make([]testInput[byte], len(sizes))

			for j, size := range sizes {
				b := make([]byte, size)
				_, _ = r.Read(b)

				l := enc.EncodedLen(size)
				tests[j] = testInput[byte]{raw: b, expected: expected[:l]}
				expected = expected[l:]
			}

			runTests(t, enc, tests)
		})
	}
}

func TestGeneric_EncodeDecode(t *testing.T) {
	for _, i := range []int{511, 512, 513, 1024, 2048} {
		t.Run(fmt.Sprintf("generic_%d", i), func(t *testing.T) {
			enc := newEncoder[uint](i, func(i int) uint {
				return uint(i)
			})

			r := newTestRand()
			sizes := []int{0, 1, 2, 7, 8, 9, 12, 39, 78, 182, 304, 790, 3000, 5029}

			for _, size := range sizes {
				b := make([]byte, size)
				_, _ = r.Read(b)

				runEncodeDecode(t, enc, b)
			}
		})
	}
}

func TestNewEncodingCopiesDict(t *testing.T) {
	dict := []byte("ab")
	enc, err := NewEncoding(dict)
	if err != nil {
		t.Fatal(err)
	}

	dict[0] = 'x'
	encoded := enc.AppendEncode(nil, []byte{0})
	if !bytes.Equal(encoded, []byte("aaaaaaaa")) {
		t.Fatalf("expected encoding to keep original dict, got %q", encoded)
	}
}

func TestDecodeRequiresPadding(t *testing.T) {
	if _, err := Base64Encoding.AppendDecode(nil, []byte("aGVsbG8")); err == nil {
		t.Fatal("expected missing padding to fail")
	}

	decoded, err := Base64Encoding.AppendDecode(nil, []byte("aGVsbG8="))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, []byte("hello")) {
		t.Fatalf("expected hello, got %q", decoded)
	}
}

func TestRawReaderRejectsTrailingBits(t *testing.T) {
	r := NewReader(Base64RawEncoding, bytes.NewBufferString("Z"))
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("expected trailing partial bits to fail")
	}
}

func TestPaddedReaderStreams(t *testing.T) {
	raw := bytes.Repeat([]byte("hello world"), 100)
	encoded := Base64Encoding.AppendEncode(nil, raw)
	src := &limitedReader[byte]{data: encoded, max: 4}
	r := NewReader(Base64Encoding, src)

	out := make([]byte, 1)
	n, err := r.Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || out[0] != raw[0] {
		t.Fatalf("expected first decoded byte %q, got n=%d out=%q", raw[:1], n, out[:n])
	}
	if src.off >= len(encoded) {
		t.Fatal("expected padded reader to return before consuming the entire encoded stream")
	}
}

func FuzzGeneric(f *testing.F) {
	sizes := []int{1, 8, 12, 39, 78, 182, 304, 790, 3000, 5029, 13020, 90389}
	for _, size := range sizes {
		f.Add(make([]byte, size))
	}

	encoders := make([]*Encoding[[]uint, uint], 0, 1024)
	for i := 2; i <= 1024; i++ {
		encoders = append(encoders, newEncoder[uint](i, func(i int) uint {
			return uint(i)
		}))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, enc := range encoders {
			runEncodeDecode[uint](t, enc, data)
		}
	})
}

func runComparisonTests(t *testing.T, name string, base encoder, enc *Encoding[[]byte, byte]) {
	t.Run(name, func(t *testing.T) {
		r := newTestRand()
		sizes := []int{1, 8, 12, 39, 78, 182, 304, 790, 3000, 5029, 13020, 90389}
		for _, size := range sizes {
			b := make([]byte, size)
			_, _ = r.Read(b)

			runTests(
				t,
				enc,
				[]testInput[byte]{
					{raw: []byte("hello world"), expected: base.AppendEncode(nil, []byte("hello world"))},
					{raw: b, expected: base.AppendEncode(nil, b)},
				},
			)
		}
	})
}

func runTests(t *testing.T, enc *Encoding[[]byte, byte], inputs []testInput[byte]) {
	for _, input := range inputs {
		encoded := enc.AppendEncode(nil, input.raw)
		if !slices.Equal(input.expected, encoded) {
			t.Errorf("expected %v, got %v", input.expected, encoded)
			t.Fail()
			return
		}

		decoded, err := enc.AppendDecode(nil, encoded)
		if err != nil {
			t.Error(err)
			t.Fail()
			return
		}

		if !slices.Equal(input.raw, decoded) {
			t.Errorf("expected %v, got %v", input.raw, decoded)
			t.Fail()
			return
		}

		var buf bytes.Buffer
		w := NewWriter(enc, &buf)
		_, _ = w.Write(input.raw)
		_ = w.Close()

		if !slices.Equal(input.expected, buf.Bytes()) {
			t.Errorf("expected %v, got %v", input.expected, buf.Bytes())
			t.Fail()
			return
		}

		r := NewReader(enc, &buf)
		b, err := io.ReadAll(r)
		if err != nil {
			t.Error(err)
			t.Fail()
			return
		}

		if !slices.Equal(input.raw, b) {
			t.Errorf("expected %v, got %v", input.raw, b)
			t.Fail()
			return
		}
	}
}

func runEncodeDecode[T comparable](t *testing.T, enc *Encoding[[]T, T], inp []byte) {
	encoded := enc.AppendEncode(nil, inp)
	decoded, err := enc.AppendDecode(nil, encoded)
	if err != nil {
		t.Error(err)
		t.Fail()
		return
	}

	if !slices.Equal(inp, decoded) {
		t.Errorf("expected %v, got %v", inp, decoded)
		t.Fail()
		return
	}

	var buf buffer[T]
	w := NewWriter(enc, &buf)
	_, _ = w.Write(inp)
	_ = w.Close()

	if !slices.Equal(encoded, buf.Values()) {
		t.Errorf("expected %v, got %v", encoded, buf)
		t.Fail()
		return
	}

	r := NewReader(enc, &buf)
	b, err := io.ReadAll(r)
	if err != nil {
		t.Error(err)
		t.Fail()
		return
	}

	if !slices.Equal(decoded, b) {
		t.Errorf("expected %v, got %v", decoded, b)
		t.Fail()
		return
	}
}

func newTestRand() *rand.Rand {
	return rand.New(rand.NewSource(1))
}

func newEncoder[T comparable](size int, fn func(i int) T) *Encoding[[]T, T] {
	dict := make([]T, size)
	for i := range len(dict) {
		dict[i] = fn(i)
	}

	var padding []T
	if len(dict) >= 512 {
		padding = []T{fn(len(dict))}
	}

	return MustNewEncoding(dict, padding...)
}
