package based

import (
	"errors"
	"fmt"
	"io"
	"math/bits"
	"slices"
)

var (
	Base32RawEncoding    = MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"))
	Base32RawHexEncoding = MustNewEncoding([]byte("0123456789ABCDEFGHIJKLMNOPQRSTUV"))
	Base64Encoding       = MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"), byte('='))
	Base64URLEncoding    = MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"), byte('='))
	Base64RawEncoding    = MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"))
	Base64RawURLEncoding = MustNewEncoding([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"))
)

type Encoding[Dict ~[]Word, Word comparable] struct {
	dict         Dict
	idx          map[Word]uint64
	size         uint64
	stepBits     uint8
	offsetToggle uint64
	padding      *Word
}

func MustNewEncoding[Dict ~[]Word, Word comparable](dict Dict, padding ...Word) *Encoding[Dict, Word] {
	enc, err := NewEncoding(dict, padding...)
	if err != nil {
		panic(err)
	}

	return enc
}

func NewEncoding[Dict ~[]Word, Word comparable](dict Dict, padding ...Word) (*Encoding[Dict, Word], error) {
	if len(dict) < 2 {
		return nil, errors.New("dict must have at least 2 words")
	} else if len(padding) > 1 {
		return nil, errors.New("padding must have at most 1 word")
	}

	enc := Encoding[Dict, Word]{
		dict:    slices.Clone(dict),
		idx:     make(map[Word]uint64),
		size:    uint64(len(dict)),
		padding: nil,
	}

	// len-1 = max index
	stepBits := bits.Len64(enc.size - 1)
	if stepBits == bits.Len64(enc.size) {
		enc.stepBits = uint8(stepBits - 1)
		enc.offsetToggle = ^uint64(0)
	} else {
		enc.stepBits = uint8(stepBits)
		enc.offsetToggle = 0
	}

	if enc.stepBits > 8 && len(padding) < 1 {
		return nil, errors.New("encoding with a dictionary of 512 or more words (more than 8 bits per token) require padding")
	}

	if len(padding) == 1 {
		padWord := padding[0]
		enc.padding = &padWord
	}

	for i, w := range enc.dict {
		if enc.padding != nil && w == *enc.padding {
			return nil, fmt.Errorf("padding word must not be part of dict; found %v at %d\n", w, i)
		}

		_, ok := enc.idx[w]
		if ok {
			return nil, fmt.Errorf("dict must consist of unique words; found duplicate %v at %d\n", w, i)
		}

		enc.idx[w] = uint64(i)
	}

	return &enc, nil
}

func (enc *Encoding[Dict, Word]) Encode(dst []Word, src []byte) {
	var index, offset uint64
	var availBits uint8

	n := enc.encode(dst, src, &index, &offset, &availBits)
	enc.finalize(dst, &index, &offset, &availBits, &n)
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

func (enc *Encoding[Dict, Word]) finalize(dst []Word, index, offset *uint64, availBits *uint8, dstIdx *int) {
	payloadLen := *dstIdx
	if *availBits > 0 {
		payloadLen++
	}
	padCount := enc.padLen(payloadLen, *availBits)

	if *availBits > 0 {
		enc.add(dst, index, offset, availBits, dstIdx)
	}

	if padCount > 0 {
		padWord := *enc.padding
		for range padCount {
			dst[*dstIdx] = padWord
			*dstIdx++
		}
	}
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
	if lenBits == 0 {
		return 0
	}

	payloadLen := lenBits / stepBits
	if lenBits%stepBits != 0 {
		payloadLen++
	}

	return payloadLen + enc.padLen(payloadLen, uint8(lenBits%stepBits))
}

func (enc *Encoding[Dict, Word]) padLen(payloadLen int, availBits uint8) int {
	if enc.padding == nil || availBits == 0 {
		return 0
	}

	if enc.stepBits <= 8 {
		blockLen := 8 / gcdInt(int(enc.stepBits), 8)
		return (blockLen - (payloadLen % blockLen)) % blockLen
	}

	return int((enc.stepBits - availBits) / 8)
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}

	return a
}

func (enc *Encoding[Dict, Word]) Decode(dst []byte, src []Word) (n int, err error) {
	var offset uint64
	var partial byte
	var partialBits uint8

	src, padCount, err := enc.trimPadding(src)
	if err != nil {
		return 0, err
	}

	n, err = enc.decode(dst, src, &offset, &partial, &partialBits)
	if err != nil {
		return n, err
	} else if partialBits > 0 && partial != 0 {
		return n, fmt.Errorf("finished with remaining partial bits [partial=%08b, partialBits=%08b]", partial, partialBits)
	}

	if enc.stepBits > 8 && padCount > 0 {
		if padCount > n {
			return n, fmt.Errorf("padding removes %d bytes from %d decoded bytes", padCount, n)
		}

		for i := n - padCount; i < n; i++ {
			if dst[i] != 0 {
				return n, fmt.Errorf("non-zero byte %08b removed by padding at decoded index %d", dst[i], i)
			}
		}

		n -= padCount
	}

	return n, nil
}

func (enc *Encoding[Dict, Word]) trimPadding(src []Word) ([]Word, int, error) {
	if enc.padding == nil {
		return src, 0, nil
	}

	padWord := *enc.padding
	padCount := 0
	for len(src) > 0 && src[len(src)-1] == padWord {
		src = src[:len(src)-1]
		padCount++
	}

	for i, w := range src {
		if w == padWord {
			return nil, 0, fmt.Errorf("padding word at index %d before end", i)
		}
	}

	return src, padCount, enc.validatePadding(len(src), padCount)
}

func (enc *Encoding[Dict, Word]) validatePadding(payloadLen, padCount int) error {
	if enc.stepBits <= 8 {
		blockLen := 8 / gcdInt(int(enc.stepBits), 8)
		expected := (blockLen - (payloadLen % blockLen)) % blockLen
		if padCount != expected {
			return fmt.Errorf("invalid padding length %d, expected %d", padCount, expected)
		}

		return nil
	}

	if padCount == 0 {
		return nil
	}

	if payloadLen == 0 {
		return errors.New("padding without encoded data")
	}

	maxDecodedLen := (payloadLen * int(enc.stepBits)) / 8
	minDecodedLen := (((payloadLen - 1) * int(enc.stepBits)) / 8) + 1
	maxPadLen := maxDecodedLen - minDecodedLen
	if padCount > maxPadLen {
		return fmt.Errorf("invalid padding length %d, expected at most %d", padCount, maxPadLen)
	}

	return nil
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
	enc        *Encoding[Dict, Word]
	w          interface{ Write(b []Word) (int, error) }
	buf        []Word
	index      uint64
	offset     uint64
	availBits  uint8
	payloadLen int
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

	encodedLen := w.enc.encodedLen(len(p), w.availBits)
	w.buf = slices.Grow(w.buf[:0], encodedLen)
	encoded := w.buf[:encodedLen]
	encodedLen = w.enc.encode(encoded, p, &w.index, &w.offset, &w.availBits)
	if encodedLen == 0 {
		return len(p), nil
	}

	written, err := w.w.Write(encoded[:encodedLen])
	if err != nil {
		return 0, err
	}
	if written != encodedLen {
		return 0, io.ErrShortWrite
	}

	w.payloadLen += encodedLen
	return len(p), nil
}

func (w *writer[Dict, Word]) Close() error {
	payloadLen := w.payloadLen
	if w.availBits > 0 {
		payloadLen++
	}
	padLen := w.enc.padLen(payloadLen, w.availBits)
	finalLen := padLen
	if w.availBits > 0 {
		finalLen++
	}
	if finalLen == 0 {
		return nil
	}

	w.buf = slices.Grow(w.buf[:0], finalLen)
	final := w.buf[:finalLen]
	n := 0
	if w.availBits > 0 {
		w.enc.add(final, &w.index, &w.offset, &w.availBits, &n)
	}
	if padLen > 0 {
		padWord := *w.enc.padding
		for range padLen {
			final[n] = padWord
			n++
		}
	}

	written, err := w.w.Write(final[:n])
	if err != nil {
		return err
	}
	if written != n {
		return io.ErrShortWrite
	}

	return nil
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
	out         []byte
	outOff      int
	err         error
	sawPadding  bool
	padCount    int
	payloadLen  int
	tail        []byte
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
	if len(p) == 0 {
		return 0, nil
	}

	for {
		if r.outOff < len(r.out) {
			n = copy(p, r.out[r.outOff:])
			r.outOff += n
			return n, nil
		}
		if r.err != nil {
			return 0, r.err
		}

		r.out = r.out[:0]
		r.outOff = 0
		if r.enc.padding != nil {
			r.err = r.fillPadded()
		} else {
			r.err = r.fillRaw()
		}
	}
}

func (r *reader[Dict, Word]) fillRaw() error {
	const readLen = 1024

	r.buf = slices.Grow(r.buf[:0], readLen)
	n, err := r.r.Read(r.buf[:readLen])
	if n > 0 {
		if decodeErr := r.decodeWords(r.buf[:n]); decodeErr != nil {
			return decodeErr
		}
	}
	if err == io.EOF {
		if closeErr := r.Close(); closeErr != nil {
			return closeErr
		}
		return io.EOF
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return io.ErrNoProgress
	}

	return nil
}

func (r *reader[Dict, Word]) fillPadded() error {
	const readLen = 1024

	r.buf = slices.Grow(r.buf[:0], readLen)
	n, err := r.r.Read(r.buf[:readLen])
	if n > 0 {
		if decodeErr := r.decodePaddedWords(r.buf[:n]); decodeErr != nil {
			return decodeErr
		}
	}
	if err == io.EOF {
		if closeErr := r.finishPadded(); closeErr != nil {
			return closeErr
		}
		return io.EOF
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return io.ErrNoProgress
	}

	return nil
}

func (r *reader[Dict, Word]) decodePaddedWords(words []Word) error {
	padWord := *r.enc.padding
	for i, word := range words {
		if word == padWord {
			r.sawPadding = true
			r.padCount++
			continue
		}
		if r.sawPadding {
			return fmt.Errorf("padding word at index %d before end", r.payloadLen)
		}

		r.payloadLen++
		if err := r.decodeWords(words[i : i+1]); err != nil {
			return err
		}
	}

	return nil
}

func (r *reader[Dict, Word]) decodeWords(words []Word) error {
	lenBits := (len(words) * int(r.enc.stepBits)) + int(r.partialBits)
	if lenBits < 8 {
		var tmp [8]byte
		n, err := r.enc.decode(tmp[:], words, &r.offset, &r.partial, &r.partialBits)
		r.appendDecoded(tmp[:n])
		return err
	}

	decoded := make([]byte, lenBits/8)
	n, err := r.enc.decode(decoded, words, &r.offset, &r.partial, &r.partialBits)
	r.appendDecoded(decoded[:n])
	return err
}

func (r *reader[Dict, Word]) appendDecoded(decoded []byte) {
	if r.enc.padding == nil || r.enc.stepBits <= 8 {
		r.out = append(r.out, decoded...)
		return
	}

	hold := int(r.enc.stepBits / 8)
	r.tail = append(r.tail, decoded...)
	if len(r.tail) <= hold {
		return
	}

	emit := len(r.tail) - hold
	r.out = append(r.out, r.tail[:emit]...)
	copy(r.tail, r.tail[emit:])
	r.tail = r.tail[:len(r.tail)-emit]
}

func (r *reader[Dict, Word]) finishPadded() error {
	if err := r.enc.validatePadding(r.payloadLen, r.padCount); err != nil {
		return err
	}
	if err := r.Close(); err != nil {
		return err
	}
	if r.enc.stepBits <= 8 {
		return nil
	}
	if r.padCount > len(r.tail) {
		return fmt.Errorf("padding removes %d bytes from %d pending decoded bytes", r.padCount, len(r.tail))
	}

	stripFrom := len(r.tail) - r.padCount
	for i := stripFrom; i < len(r.tail); i++ {
		if r.tail[i] != 0 {
			return fmt.Errorf("non-zero byte %08b removed by padding at decoded index %d", r.tail[i], i)
		}
	}

	r.out = append(r.out, r.tail[:stripFrom]...)
	r.tail = r.tail[:0]
	return nil
}

func (r *reader[Dict, Word]) Close() error {
	if r.partialBits > 0 && r.partial != 0 {
		return fmt.Errorf("finished with remaining partial bits (0<%d<8) [%0b]", r.partialBits, r.partial)
	}

	return nil
}
