package based

import (
	"encoding/base32"
	"encoding/base64"
	"math/rand/v2"
	"testing"
)

var (
	benchEncoded []byte
	benchDecoded []byte
)

func BenchmarkAppendEncode(b *testing.B) {
	src := benchmarkInput(32 * 1024)

	benchAppendEncode(b, "base64/std", base64.StdEncoding, Base64Encoding, src)
	benchAppendEncode(b, "base64/raw", base64.RawStdEncoding, Base64RawEncoding, src)
	benchAppendEncode(b, "base32/std", base32.StdEncoding, MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"), byte('=')), src)
	benchAppendEncode(b, "base32/raw", base32.StdEncoding.WithPadding(base32.NoPadding), Base32RawEncoding, src)
}

func BenchmarkAppendDecode(b *testing.B) {
	src := benchmarkInput(32 * 1024)

	benchAppendDecode(b, "base64/std", base64.StdEncoding, Base64Encoding, src)
	benchAppendDecode(b, "base64/raw", base64.RawStdEncoding, Base64RawEncoding, src)
	benchAppendDecode(b, "base32/std", base32.StdEncoding, MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"), byte('=')), src)
	benchAppendDecode(b, "base32/raw", base32.StdEncoding.WithPadding(base32.NoPadding), Base32RawEncoding, src)
}

func benchAppendEncode(b *testing.B, name string, std interface {
	AppendEncode(dst, src []byte) []byte
}, enc *Encoding[[]byte, byte], src []byte) {
	b.Run(name+"/stdlib", func(b *testing.B) {
		dst := make([]byte, 0, stdEncodedLen(name, len(src)))
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			dst = std.AppendEncode(dst[:0], src)
		}

		benchEncoded = dst
	})

	b.Run(name+"/based", func(b *testing.B) {
		dst := make([]byte, 0, enc.EncodedLen(len(src)))
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			dst = enc.AppendEncode(dst[:0], src)
		}

		benchEncoded = dst
	})
}

func benchAppendDecode(b *testing.B, name string, std interface {
	AppendDecode(dst, src []byte) ([]byte, error)
}, enc *Encoding[[]byte, byte], raw []byte) {
	stdEncoded := stdbaseEncoded(name, raw)
	basedEncoded := enc.AppendEncode(nil, raw)

	b.Run(name+"/stdlib", func(b *testing.B) {
		dst := make([]byte, 0, len(raw))
		b.SetBytes(int64(len(raw)))
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var err error
			dst, err = std.AppendDecode(dst[:0], stdEncoded)
			if err != nil {
				b.Fatal(err)
			}
		}

		benchDecoded = dst
	})

	b.Run(name+"/based", func(b *testing.B) {
		dst := make([]byte, 0, len(raw))
		b.SetBytes(int64(len(raw)))
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var err error
			dst, err = enc.AppendDecode(dst[:0], basedEncoded)
			if err != nil {
				b.Fatal(err)
			}
		}

		benchDecoded = dst
	})
}

func benchmarkInput(size int) []byte {
	src := make([]byte, size)
	r := rand.NewChaCha8([32]byte{})
	for i := range src {
		src[i] = byte(r.Uint64())
	}
	return src
}

func stdbaseEncoded(name string, src []byte) []byte {
	switch name {
	case "base64/std":
		return base64.StdEncoding.AppendEncode(nil, src)
	case "base64/raw":
		return base64.RawStdEncoding.AppendEncode(nil, src)
	case "base32/std":
		return base32.StdEncoding.AppendEncode(nil, src)
	case "base32/raw":
		return base32.StdEncoding.WithPadding(base32.NoPadding).AppendEncode(nil, src)
	default:
		panic("unknown benchmark encoding: " + name)
	}
}

func stdEncodedLen(name string, n int) int {
	switch name {
	case "base64/std":
		return base64.StdEncoding.EncodedLen(n)
	case "base64/raw":
		return base64.RawStdEncoding.EncodedLen(n)
	case "base32/std":
		return base32.StdEncoding.EncodedLen(n)
	case "base32/raw":
		return base32.StdEncoding.WithPadding(base32.NoPadding).EncodedLen(n)
	default:
		panic("unknown benchmark encoding: " + name)
	}
}
