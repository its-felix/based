package based

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"io"
	"math/rand/v2"
	"slices"
	"testing"
)

type testInput[T any] struct {
	raw      []byte
	expected []T
}

type encoder interface {
	AppendEncode(dst, src []byte) []byte
}

func TestAll(t *testing.T) {
	runComparisonTests(t, base32.StdEncoding.WithPadding(base32.NoPadding), RawStdBase32Bytes)
	runComparisonTests(t, base32.HexEncoding.WithPadding(base32.NoPadding), RawHexBase32Bytes)
	runComparisonTests(t, base64.RawStdEncoding, RawStdBase64Bytes)
	runComparisonTests(t, base64.RawURLEncoding, RawURLBase64Bytes)
}

func runComparisonTests(t *testing.T, base encoder, enc *Encoding[[]byte, byte]) {
	r := rand.NewChaCha8([32]byte{})
	sizes := []int{1, 8, 12, 39, 78, 182, 304, 790, 3000, 5029, 13020}
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
