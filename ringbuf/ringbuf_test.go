package ringbuf

import (
	"bytes"
	"testing"
)

const (
	doRead = iota
	doWrite
)

type rbtest struct {
	op        int
	data      string
	bufSize   int
	ret       int
	totalSize int
}

func TestRingbuf(t *testing.T) {
	r := New(5)

	tab := []rbtest{
		{doWrite, "abc", 0, 3, 3},
		{doWrite, "d", 0, 1, 4},
		{doRead, "abcd", 4, 4, 4},
		{doWrite, "ef", 0, 2, 5},
		{doRead, "bcdef", 5, 5, 5},
		{doWrite, "abcdefg", 0, 5, 5},
		{doWrite, "fg", 0, 2, 5},
		{doRead, "efgfg", 5, 5, 5},
		{doWrite, "abracadabra", 0, 11, 5},
		{doRead, "dabra", 5, 5, 5},
	}

	for _, step := range tab {
		switch step.op {
		case doRead:
			b := make([]byte, step.bufSize)
			n := r.Read(b)
			t.Logf("r.Read(%d) = %d, %#v", step.bufSize, step.ret, string(b[:n]))
			if n != step.ret {
				t.Errorf("r.Read(%#v) = %d, want %d", b, n, step.ret)
			}
			if !bytes.Equal(b[:n], []byte(step.data)) {
				t.Errorf("b = %#v, want %#v", string(b[:n]), step.data)
			}

		case doWrite:
			r.Write([]byte(step.data))
			t.Logf("r.Write(%#v) = %d", string(step.data), step.ret)
		}
		if r.Size() != step.totalSize {
			t.Errorf("r.Size() = %d, want %d", r.Size(), step.totalSize)
		}
		t.Logf("%#v", r)
	}
}
