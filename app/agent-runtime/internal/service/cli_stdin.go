package service

import (
	"io"
	"sync"
)

// shiftTabReader intercepts Shift+Tab escape sequences before readline sees them.
type shiftTabReader struct {
	inner      io.Reader
	onShiftTab func()
	mu         sync.Mutex
	buf        []byte
}

func newShiftTabReader(inner io.Reader, onShiftTab func()) *shiftTabReader {
	return &shiftTabReader{inner: inner, onShiftTab: onShiftTab}
}

func (r *shiftTabReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	tmp := make([]byte, len(p))
	if len(tmp) < 32 {
		tmp = make([]byte, 32)
	}
	n, err := r.inner.Read(tmp)
	if n <= 0 {
		return 0, err
	}
	data := tmp[:n]

	for {
		consumed, shiftTab := matchShiftTab(data)
		if !shiftTab {
			break
		}
		if r.onShiftTab != nil {
			r.onShiftTab()
		}
		data = data[consumed:]
		if len(data) == 0 {
			return 0, nil
		}
	}

	copied := copy(p, data)
	if copied < len(data) {
		r.buf = append(r.buf[:0], data[copied:]...)
	}
	if copied == 0 && err != nil {
		return 0, err
	}
	return copied, err
}

func matchShiftTab(b []byte) (consumed int, ok bool) {
	if len(b) < 2 || b[0] != '\x1b' {
		return 0, false
	}
	switch b[1] {
	case 'Z', 'z':
		return 2, true
	case '[':
		i := 2
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
		if i >= len(b) {
			return 0, false
		}
		if b[i] == ';' {
			i++
			for i < len(b) && b[i] >= '0' && b[i] <= '9' {
				i++
			}
			if i >= len(b) {
				return 0, false
			}
		}
		if b[i] == 'Z' || b[i] == 'z' {
			return i + 1, true
		}
	}
	return 0, false
}

type shiftTabReadCloser struct {
	*shiftTabReader
}

func (r *shiftTabReadCloser) Close() error {
	return nil
}
