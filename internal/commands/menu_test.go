package commands

import (
	"io"
	"testing"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestReadTTYByteWaitsAfterTemporaryEOF(t *testing.T) {
	reads := 0
	r := readerFunc(func(p []byte) (int, error) {
		reads++
		if reads == 1 {
			return 0, io.EOF
		}
		p[0] = 'q'
		return 1, nil
	})

	b, err := readTTYByte(r, 0, nil)
	if err != nil || b != 'q' || reads != 2 {
		t.Fatalf("got byte %q, error %v, reads %d", b, err, reads)
	}
}
