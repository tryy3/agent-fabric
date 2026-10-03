package sandboxcore

// CappedBuffer is an io.Writer that keeps at most Max bytes and silently
// drains the rest, so a chatty child process never blocks on a full pipe.
// Max <= 0 means unlimited.
type CappedBuffer struct {
	Max       int
	buf       []byte
	truncated bool
}

// Write implements io.Writer. It always reports the full length as written.
func (b *CappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Max <= 0 {
		b.buf = append(b.buf, p...)
		return n, nil
	}
	room := b.Max - len(b.buf)
	if room <= 0 {
		b.truncated = b.truncated || n > 0
		return n, nil
	}
	if len(p) > room {
		p = p[:room]
		b.truncated = true
	}
	b.buf = append(b.buf, p...)
	return n, nil
}

// Bytes returns the retained output.
func (b *CappedBuffer) Bytes() []byte { return b.buf }

// Truncated reports whether any output was discarded.
func (b *CappedBuffer) Truncated() bool { return b.truncated }

// CapBytes truncates data to max bytes (max <= 0 means unlimited) and reports
// whether anything was dropped.
func CapBytes(data []byte, max int) ([]byte, bool) {
	if max <= 0 || len(data) <= max {
		return data, false
	}
	return data[:max], true
}
