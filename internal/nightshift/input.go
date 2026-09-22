package nightshift

import (
	"bytes"
	"io"
	"time"
)

type keyKind int

const (
	keyIgnore keyKind = iota
	keyTrigger
	keyEnter
	keyQuit
	keyEOF
)

const escWait = 30 * time.Millisecond

// deadlineReader is the raw terminal. Key bytes stay inside the reader.
type deadlineReader interface {
	Read(p []byte) (int, error)
	SetReadDeadline(t time.Time) error
}

type byteReader struct {
	f       deadlineReader
	buf     []byte
	i       int
	scratch []byte
}

// nextKey classifies one event. The bytes are discarded and are not returned.
func (r *byteReader) nextKey() (keyKind, error) {
	b, err := r.take()
	if err != nil {
		return keyEOF, err
	}
	switch b {
	case 0x03:
		return keyQuit, nil
	case '\r':
		if next, ok := r.peek(0); ok && next == '\n' {
			_, _ = r.take()
		}
		return keyEnter, nil
	case '\n':
		return keyEnter, nil
	case 0x1b:
		return r.escapeKind(), nil
	}
	if b < 0x20 || b == 0x7f {
		return keyIgnore, nil
	}
	if b < 0x80 {
		return keyTrigger, nil
	}
	if b&0xc0 == 0x80 {
		return keyIgnore, nil
	}
	r.skipUTF8(b)
	return keyTrigger, nil
}

func (r *byteReader) escapeKind() keyKind {
	b, ok := r.peek(escWait)
	if !ok || (b != '[' && b != 'O') {
		return keyIgnore
	}
	_, _ = r.take()
	if b == 'O' {
		if _, ok = r.peek(escWait); ok {
			_, _ = r.take()
		}
		return keyIgnore
	}
	seq := r.readCSI()
	if bytes.Equal(seq, []byte("200~")) {
		for i := range seq {
			seq[i] = 0
		}
		r.discardPaste()
		return keyTrigger
	}
	for i := range seq {
		seq[i] = 0
	}
	return keyIgnore
}

func (r *byteReader) readCSI() []byte {
	seq := make([]byte, 0, 8)
	for len(seq) < 48 {
		b, ok := r.peek(escWait)
		if !ok {
			return seq
		}
		_, _ = r.take()
		seq = append(seq, b)
		if b >= 0x40 && b <= 0x7e {
			return seq
		}
	}
	return seq
}

func (r *byteReader) discardPaste() {
	marker := []byte{0x1b, '[', '2', '0', '1', '~'}
	var tail [6]byte
	n := 0
	for {
		b, err := r.take()
		if err != nil {
			break
		}
		if n < len(tail) {
			tail[n] = b
			n++
		} else {
			copy(tail[:], tail[1:])
			tail[len(tail)-1] = b
		}
		if n == len(marker) && bytes.Equal(tail[:], marker) {
			break
		}
	}
	for i := range tail {
		tail[i] = 0
	}
}

func (r *byteReader) skipUTF8(lead byte) {
	need := 0
	switch {
	case lead&0xE0 == 0xC0:
		need = 1
	case lead&0xF0 == 0xE0:
		need = 2
	case lead&0xF8 == 0xF0:
		need = 3
	default:
		return
	}
	for i := 0; i < need; i++ {
		b, ok := r.peek(escWait)
		if !ok || b&0xC0 != 0x80 {
			return
		}
		_, _ = r.take()
	}
}

func (r *byteReader) take() (byte, error) {
	if r.i >= len(r.buf) {
		if err := r.fill(0); err != nil {
			return 0, err
		}
	}
	b := r.buf[r.i]
	r.buf[r.i] = 0
	r.i++
	return b, nil
}

func (r *byteReader) peek(wait time.Duration) (byte, bool) {
	if r.i < len(r.buf) {
		return r.buf[r.i], true
	}
	if wait <= 0 {
		return 0, false
	}
	if err := r.fill(wait); err != nil || r.i >= len(r.buf) {
		return 0, false
	}
	return r.buf[r.i], true
}

func (r *byteReader) fill(wait time.Duration) error {
	if cap(r.scratch) < 256 {
		r.scratch = make([]byte, 256)
	}
	scratch := r.scratch[:256]
	if wait > 0 {
		if err := r.f.SetReadDeadline(time.Now().Add(wait)); err != nil {
			return err
		}
		defer func() { _ = r.f.SetReadDeadline(time.Time{}) }()
	}
	n, err := r.f.Read(scratch)
	if n > 0 {
		fresh := make([]byte, n)
		copy(fresh, scratch[:n])
		for i := 0; i < n; i++ {
			scratch[i] = 0
		}
		r.buf = fresh
		r.i = 0
		return nil
	}
	for i := range scratch {
		scratch[i] = 0
	}
	if err != nil {
		return err
	}
	return io.EOF
}
