package fch

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Reader walks a byte buffer using Valheim's ZPackage primitive encodings.
type Reader struct {
	Data []byte
	Pos  int
}

func NewReader(data []byte) *Reader {
	return &Reader{Data: data}
}

func (r *Reader) I32() int32 {
	v := int32(binary.LittleEndian.Uint32(r.Data[r.Pos:]))
	r.Pos += 4
	return v
}

func (r *Reader) I64() int64 {
	v := int64(binary.LittleEndian.Uint64(r.Data[r.Pos:]))
	r.Pos += 8
	return v
}

func (r *Reader) U16() uint16 {
	v := binary.LittleEndian.Uint16(r.Data[r.Pos:])
	r.Pos += 2
	return v
}

func (r *Reader) U8() uint8 {
	v := r.Data[r.Pos]
	r.Pos++
	return v
}

func (r *Reader) F32() float32 {
	bits := binary.LittleEndian.Uint32(r.Data[r.Pos:])
	r.Pos += 4
	return math.Float32frombits(bits)
}

func (r *Reader) Bool() bool {
	return r.U8() != 0
}

// NumItems reads Valheim's 7-bit-encoded variable length count.
func (r *Reader) NumItems() int {
	b := r.U8()
	if b&128 != 0 {
		b2 := r.U8()
		return (int(b&127) << 8) | int(b2)
	}
	return int(b)
}

func (r *Reader) String() string {
	n := r.NumItems()
	s := string(r.Data[r.Pos : r.Pos+n])
	r.Pos += n
	return s
}

func (r *Reader) Vec3() [3]float32 {
	return [3]float32{r.F32(), r.F32(), r.F32()}
}

func (r *Reader) ByteArray() []byte {
	n := int(r.I32())
	b := r.Data[r.Pos : r.Pos+n]
	r.Pos += n
	return b
}

func (r *Reader) ZDOID() {
	r.I64()
	r.I32()
}

func (r *Reader) Skip(n int) {
	r.Pos += n
}

func (r *Reader) Range(start int) [2]int {
	return [2]int{start, r.Pos}
}

func (r *Reader) checkBounds(n int) error {
	if r.Pos+n > len(r.Data) {
		return fmt.Errorf("out of bounds read at pos=%d len=%d want=%d", r.Pos, len(r.Data), n)
	}
	return nil
}
