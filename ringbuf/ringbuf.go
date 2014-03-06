// License: Apache License 2.0
// Author: David Anderson
// URL: https://code.google.com/p/curvecp/source/browse/ringbuf/ringbuf.go

// Package ringbuf implements a byte ring buffer. The interface is
// close to that of an io.ReadWriter, but note that the semantics
// differ in significant ways, because this ringbuf is an
// implementation detail of the curvecp package, and it was more
// convenient like this.
package ringbuf

type Ringbuf struct {
	buf         []byte
	start, size int
}

// New creates a new ring buffer of the given size.
func New(size int) *Ringbuf {
	return &Ringbuf{make([]byte, size), 0, 0}
}

// Write appends all the bytes in b to the buffer, looping and overwriting
// as needed, while incrementing the start to point to the start of the
// buffer.
func (r *Ringbuf) Write(b []byte) {
	for len(b) > 0 {
		start := (r.start + r.size) % len(r.buf)

		// Copy as much as we can from where we are
		n := copy(r.buf[start:], b)
		b = b[n:]

		// Are we already at capacity? Move the start an appropriate
		// distance forward depending on how much we copied.
		if r.size >= len(r.buf) {
			if n <= len(r.buf) {
				r.start += n
				if r.start >= len(r.buf) {
					r.start = 0
				}
			} else {
				r.start = 0
			}
		}
		r.size += n
		// Size can't exceed the capacity
		if r.size > cap(r.buf) {
			r.size = cap(r.buf)
		}
	}
}

// Read reads as many bytes as possible from the ring buffer into
// b. Returns the number of bytes read.
func (r *Ringbuf) Read(b []byte) int {
	read := 0
	size := r.size
	start := r.start
	for len(b) > 0 && size > 0 {
		end := start + size
		if end > len(r.buf) {
			end = len(r.buf)
		}
		n := copy(b, r.buf[start:end])
		size -= n
		read += n
		b = b[n:]
		start = (start + n) % len(r.buf)
	}
	return read
}

func (r *Ringbuf) Size() int {
	return r.size
}
