package mhtml

import "testing"

// chunks drains s, rendering each chunk as a string.
func chunks(s *scanner) []string {
	var out []string
	for c, ok := s.nextChunk(); ok; c, ok = s.nextChunk() {
		out = append(out, string(c))
	}
	return out
}

func assertChunks(t *testing.T, s *scanner, want ...string) {
	t.Helper()
	got := chunks(s)
	if len(got) != len(want) {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("chunks = %q, want %q", got, want)
		}
	}
	if _, ok := s.nextChunk(); ok {
		t.Fatal("scanner yielded a chunk after EOF")
	}
}

func TestSeparatorMidBufferReturnsBytesBeforeIt(t *testing.T) {
	assertChunks(t, newScanner([]byte("abc\r\ndef"), crlf), "abc", "def")
}

func TestSeparatorAtStartYieldsEmptyChunk(t *testing.T) {
	assertChunks(t, newScanner([]byte("\r\nabc"), crlf), "", "abc")
}

func TestSeparatorAtEndHasNoTrailingEmptyChunk(t *testing.T) {
	assertChunks(t, newScanner([]byte("abc\r\n"), crlf), "abc")
}

func TestAbsentSeparatorYieldsRemainderOnceThenNone(t *testing.T) {
	assertChunks(t, newScanner([]byte("abc"), crlf), "abc")
}

func TestEmptyDataIsEOFImmediately(t *testing.T) {
	assertChunks(t, newScanner(nil, crlf))
}

func TestAdjacentSeparatorsYieldEmptyChunksBetween(t *testing.T) {
	assertChunks(t, newScanner([]byte("abc\r\n\r\ndef"), crlf), "abc", "", "def")
}

func TestSetSeparatorMidStreamSwitchesDelimiter(t *testing.T) {
	// Read the first CRLF-delimited line, then switch to a raw boundary.
	s := newScanner([]byte("header\r\nbody--BOUNDARY tail"), crlf)
	if c, _ := s.nextChunk(); string(c) != "header" {
		t.Fatalf("first chunk = %q", c)
	}
	s.sep = []byte("--BOUNDARY")
	assertChunks(t, s, "body", " tail")
}

func TestPeekReturnsUpToNWithoutAdvancing(t *testing.T) {
	s := newScanner([]byte("--boundary"), crlf)
	if p := s.peek(2); string(p) != "--" {
		t.Fatalf("peek = %q", p)
	}
	// No advance: a subsequent read still sees the whole buffer.
	if p := s.peek(2); string(p) != "--" {
		t.Fatalf("peek = %q", p)
	}
	assertChunks(t, s, "--boundary")
}

func TestPeekShorterThanNReturnsWhatIsAvailable(t *testing.T) {
	if p := newScanner([]byte("ab"), crlf).peek(10); string(p) != "ab" {
		t.Fatalf("peek = %q", p)
	}
}

func TestNULBytesArePreservedInChunks(t *testing.T) {
	assertChunks(t, newScanner([]byte("a\x00b\r\nc\x00"), crlf), "a\x00b", "c\x00")
}

func TestChunkCapacityCannotClobberInput(t *testing.T) {
	data := []byte("ab\r\ncd")
	c, _ := newScanner(data, crlf).nextChunk()
	_ = append(c, 'X')
	if string(data) != "ab\r\ncd" {
		t.Fatalf("append to chunk modified input: %q", data)
	}
}

func TestDecodeUTF8PassesValidUTF8Through(t *testing.T) {
	if d := decodeUTF8Latin1([]byte("hello")); d != "hello" {
		t.Fatalf("got %q", d)
	}
}

func TestDecodeLatin1FallbackMapsBytesToCodePoints(t *testing.T) {
	if d := decodeUTF8Latin1([]byte("caf\xe9")); d != "café" {
		t.Fatalf("got %q", d)
	}
}
