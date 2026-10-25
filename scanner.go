package mhtml

import (
	"bytes"
	"unicode/utf8"
)

var crlf = []byte("\r\n")

// scanner reads a contiguous []byte as a sequence of chunks delimited by a
// runtime-swappable separator. The parts phase toggles the separator between
// CRLF (text framing) and the raw MIME boundary (binary framing).
type scanner struct {
	data []byte
	pos  int
	sep  []byte
}

func newScanner(data, sep []byte) *scanner {
	return &scanner{data: data, sep: sep}
}

// nextChunk returns the bytes up to (but excluding) the next separator,
// advancing past that separator. When no separator remains, the final
// unterminated remainder is returned exactly once; every call thereafter
// reports false. A separator sitting exactly at the cursor yields an empty
// chunk — the load-bearing "empty header line" vs. "EOF" distinction.
func (s *scanner) nextChunk() ([]byte, bool) {
	if s.pos >= len(s.data) {
		return nil, false
	}
	rest := s.data[s.pos:]
	if rel := bytes.Index(rest, s.sep); rel >= 0 {
		s.pos += rel + len(s.sep)
		// Cap capacity so a caller appending to a chunk can't clobber the input.
		return rest[:rel:rel], true
	}
	s.pos = len(s.data)
	return rest, true
}

// peek returns up to n bytes at the cursor without advancing; fewer when the
// buffer is shorter.
func (s *scanner) peek(n int) []byte {
	return s.data[s.pos:min(s.pos+n, len(s.data))]
}

// remaining returns the bytes from the cursor to the end, without advancing.
func (s *scanner) remaining() []byte {
	return s.data[s.pos:]
}

// decodeUTF8Latin1 decodes b as UTF-8, falling back to a 1:1 Latin-1 mapping
// (each byte to the identical U+00.. code point) when b is not valid UTF-8.
// Never fails. Header lines and text bodies flow through this.
func decodeUTF8Latin1(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}
