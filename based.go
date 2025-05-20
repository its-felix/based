package based

import (
	"fmt"
	"io"
	"math/bits"
	"slices"
)

var (
	RawStdBase32Bytes = NewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"))
	RawHexBase32Bytes = NewEncoding([]byte("0123456789ABCDEFGHIJKLMNOPQRSTUV"))
	RawStdBase64Bytes = NewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"))
	RawURLBase64Bytes = NewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"))
)

type Encoding[Dict ~[]Word, Word comparable] struct {
	dict         Dict
	idx          map[Word]uint64
	size         uint64
	stepBits     uint8
	offsetToggle uint64
}

func NewEncoding[Dict ~[]Word, Word comparable](dict Dict) *Encoding[Dict, Word] {
	if len(dict) < 2 {
		panic("dict must have at least 2 words")
	}

	enc := Encoding[Dict, Word]{
		dict: dict,
		idx:  make(map[Word]uint64),
		size: uint64(len(dict)),
	}

	for i, w := range dict {
		_, ok := enc.idx[w]
		if ok {
			panic(fmt.Sprintf("dict must consist of unique words; found duplicate %v at %d\n", w, i))
		}

		enc.idx[w] = uint64(i)
	}

	// len-1 = max index
	size := bits.Len64(enc.size - 1)
	if size == bits.Len64(enc.size) {
		enc.stepBits = uint8(size - 1)
		enc.offsetToggle = ^uint64(0)
	} else {
		enc.stepBits = uint8(size)
		enc.offsetToggle = 0
	}

	return &enc
}

func (enc *Encoding[Dict, Word]) Encode(dst []Word, src []byte) {
	var index, offset uint64
	var availBits uint8

	n := enc.encode(dst, src, &index, &offset, &availBits)
	if availBits > 0 {
		enc.add(dst, &index, &offset, &availBits, &n)
	}
}

func (enc *Encoding[Dict, Word]) encode(dst []Word, src []byte, index, offset *uint64, availBits *uint8) (n int) {
	if len(src) == 0 {
		return 0
	}

	_ = enc.stepBits
	_, _, _ = *index, *offset, *availBits

	for len(src) > 0 {
		read := src[0] // read byte from src
		src = src[1:]  // advance src

		remReadBits := uint8(8)
		for remReadBits > 0 {
			readBits := min(enc.stepBits-*availBits, remReadBits)

			*index <<= readBits                                // make space for the bits we're going to read
			*index |= uint64(read >> (remReadBits - readBits)) // add the bits
			*availBits += readBits

			read &= (1 << (remReadBits - readBits)) - 1 // remove the bits that have been read
			remReadBits -= readBits                     // decrease remaining read bits

			if *availBits == enc.stepBits {
				enc.add(dst, index, offset, availBits, &n)
			}
		}
	}

	return n
}

func (enc *Encoding[Dict, Word]) add(dst []Word, index, offset *uint64, availBits *uint8, dstIdx *int) {
	*index <<= enc.stepBits - *availBits
	*index += *offset

	if *index >= enc.size {
		*index -= enc.size
	}

	dst[*dstIdx] = enc.dict[*index]
	*dstIdx++
	*offset = *index & enc.offsetToggle

	*index = 0
	*availBits = 0
}

func (enc *Encoding[Dict, Word]) AppendEncode(dst []Word, src []byte) []Word {
	n := enc.EncodedLen(len(src))
	dst = slices.Grow(dst, n)
	enc.Encode(dst[len(dst):][:n], src)
	return dst[:len(dst)+n]
}

func (enc *Encoding[Dict, Word]) EncodedLen(n int) int {
	return enc.encodedLen(n, 0)
}

func (enc *Encoding[Dict, Word]) encodedLen(n int, availBits uint8) int {
	lenBits := (n * 8) + int(availBits)
	stepBits := int(enc.stepBits)

	if lenBits%stepBits == 0 {
		return lenBits / stepBits
	}

	return (lenBits / stepBits) + 1
}

func (enc *Encoding[Dict, Word]) Decode(dst []byte, src []Word) (n int, err error) {
	var offset uint64
	var partial byte
	var partialBits uint8

	n, err = enc.decode(dst, src, &offset, &partial, &partialBits)
	if err != nil {
		return n, err
	} else if partialBits > 0 && partial != 0 {
		return n, fmt.Errorf("finished with remaining partial bits [partial=%08b, partialBits=%08b]", partial, partialBits)
	}

	return n, nil
}

func (enc *Encoding[Dict, Word]) decode(dst []byte, src []Word, offset *uint64, partial *byte, partialBits *uint8) (n int, err error) {
	_, _ = enc.dict, enc.stepBits
	_, _, _ = *offset, *partial, *partialBits

	for i, w := range src {
		index, ok := enc.idx[w]
		if !ok {
			return n, fmt.Errorf("unknown word %v at index %d", w, i)
		}

		temp := index

		if *offset <= index {
			index -= *offset
		} else {
			index = enc.size - (*offset - index)
		}

		remReadBits := enc.stepBits
		for remReadBits > 0 {
			readBits := min(remReadBits, 8-*partialBits)

			*partial <<= readBits
			*partial |= byte(index >> (remReadBits - readBits))
			*partialBits += readBits

			index &= (1 << (remReadBits - readBits)) - 1
			remReadBits -= readBits

			if *partialBits == 8 {
				dst[n] = *partial
				n++
				*partial, *partialBits = 0, 0
			}
		}

		*offset = temp & enc.offsetToggle
	}

	return n, nil
}

func (enc *Encoding[Dict, Word]) AppendDecode(dst []byte, src []Word) ([]byte, error) {
	n := enc.DecodedLen(len(src))
	dst = slices.Grow(dst, n)
	read, err := enc.Decode(dst[len(dst):][:n], src)
	return dst[:len(dst)+read], err
}

func (enc *Encoding[Dict, Word]) DecodedLen(n int) int {
	lenBits := n * int(enc.stepBits)
	return lenBits / 8
}

type writer[Dict ~[]Word, Word comparable] struct {
	enc       *Encoding[Dict, Word]
	w         interface{ Write(b []Word) (int, error) }
	buf       []Word
	index     uint64
	offset    uint64
	availBits uint8
}

func NewWriter[Dict ~[]Word, Word comparable](enc *Encoding[Dict, Word], w interface{ Write(b []Word) (int, error) }) io.WriteCloser {
	return &writer[Dict, Word]{
		enc: enc,
		w:   w,
	}
}

func (w *writer[Dict, Word]) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	n = w.enc.encodedLen(len(p), w.availBits)
	w.buf = slices.Grow(w.buf, n)
	n = w.enc.encode(w.buf[len(w.buf):][:n], p, &w.index, &w.offset, &w.availBits)

	return w.w.Write(w.buf[:len(w.buf)+n])
}

func (w *writer[Dict, Word]) Close() error {
	var err error
	if w.buf != nil && w.availBits > 0 {
		var n int
		w.enc.add(w.buf[len(w.buf):][:1], &w.index, &w.offset, &w.availBits, &n)
		_, err = w.w.Write(w.buf[:len(w.buf)+n])
	}

	return err
}

type reader[Dict ~[]Word, Word comparable] struct {
	enc *Encoding[Dict, Word]
	r   interface {
		Read(p []Word) (n int, err error)
	}
	buf         []Word
	offset      uint64
	partial     byte
	partialBits uint8
}

func NewReader[Dict ~[]Word, Word comparable](enc *Encoding[Dict, Word], r interface {
	Read(p []Word) (n int, err error)
}) io.ReadCloser {
	return &reader[Dict, Word]{
		enc: enc,
		r:   r,
	}
}

func (r *reader[Dict, Word]) Read(p []byte) (n int, err error) {
	n = r.enc.encodedLen(len(p), 0)
	if n > 0 && r.partialBits > 0 {
		n--
	}

	r.buf = slices.Grow(r.buf, n)

	n, err = r.r.Read(r.buf[len(r.buf):][:n])
	if err != nil {
		return n, err
	}

	return r.enc.decode(p, r.buf[:len(r.buf)+n], &r.offset, &r.partial, &r.partialBits)
}

func (r *reader[Dict, Word]) Close() error {
	if r.partialBits > 0 && r.partial != 0 {
		return fmt.Errorf("finished with remaining partial bits (0<%d<8) [%0b]", r.partialBits, r.partial)
	}

	return nil
}
