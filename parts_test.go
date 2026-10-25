package mhtml

import (
	"errors"
	"testing"
)

// onePart drives parseNextPart over a body region with the given boundaries
// ("" meaning none).
func onePart(t *testing.T, body, hdr, partB, docB string) (*Part, bool, error) {
	t.Helper()
	var pb, db []byte
	if partB != "" {
		pb, db = []byte(partB), []byte(docB)
	}
	return parseNextPart(newScanner([]byte(body), crlf), parseHeader(t, hdr), pb, db)
}

func mustBody(t *testing.T, p *Part, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("part: %v", err)
	}
	b, err := p.Body()
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	return string(b)
}

const (
	binaryHdr = "Content-Transfer-Encoding: binary\r\n\r\n"
	sevenHdr  = "Content-Transfer-Encoding: 7bit\r\n\r\n"
)

func TestBinaryWithoutBoundaryIsError(t *testing.T) {
	// Non-multipart root + binary encoding: no boundary to delimit it.
	if _, _, err := onePart(t, "anything", binaryHdr, "", ""); !errors.Is(err, ErrBinaryPartWithoutBoundary) {
		t.Fatalf("err = %v", err)
	}
}

func TestBinaryStripsOneTrailingCRLFBeforeBoundary(t *testing.T) {
	p, eoa, err := onePart(t, "data\r\n--B\r\n", binaryHdr, "--B", "--B--")
	if got := mustBody(t, p, err); got != "data" || eoa {
		t.Fatalf("body = %q, eoa = %v", got, eoa)
	}
}

func TestBinaryWithoutTrailingCRLFKeepsAllBytes(t *testing.T) {
	// Bug-compat: a binary part joined directly to the boundary (no CRLF).
	p, _, err := onePart(t, "data--B\r\n", binaryHdr, "--B", "--B--")
	if got := mustBody(t, p, err); got != "data" {
		t.Fatalf("body = %q", got)
	}
}

func TestBinaryDocumentBoundarySetsEndOfArchive(t *testing.T) {
	p, eoa, err := onePart(t, "--B--\r\n", binaryHdr, "--B", "--B--")
	if got := mustBody(t, p, err); got != "" || !eoa {
		t.Fatalf("body = %q, eoa = %v", got, eoa)
	}
}

func TestBinaryBodyCannotClobberInput(t *testing.T) {
	data := []byte("data\r\n--B\r\n")
	p, _, err := parseNextPart(newScanner(data, crlf), parseHeader(t, binaryHdr), []byte("--B"), []byte("--B--"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.Body()
	_ = append(b, 'X')
	if string(data) != "data\r\n--B\r\n" {
		t.Fatalf("append to body modified input: %q", data)
	}
}

func TestBinaryErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       error
	}{
		// The scanner is already at EOF: an early return.
		{"empty body at EOF is unterminated", "", ErrUnterminatedPart},
		// No boundary before EOF: the whole remainder becomes the content, so
		// the failure surfaces at the boundary-tail peek.
		{"no boundary in remainder is malformed", "data no boundary", ErrMalformedBoundary},
		// "--B" is followed by "junk" rather than the expected CRLF line.
		{"non-empty tail after part boundary is malformed", "data--Bjunk\r\n", ErrMalformedBoundary},
		// The boundary sits at the very end: peek(2) sees only one byte.
		{"fewer than two bytes after boundary is malformed", "data--B\n", ErrMalformedBoundary},
	} {
		if _, _, err := onePart(t, tc.body, binaryHdr, "--B", "--B--"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestSevenBitJoinsLinesWithoutCRLF(t *testing.T) {
	p, eoa, err := onePart(t, "123\r\nabc\r\n--B--\r\n", sevenHdr, "--B", "--B--")
	if got := mustBody(t, p, err); got != "123abc" || !eoa {
		t.Fatalf("body = %q, eoa = %v", got, eoa)
	}
}

func TestQuotedPrintableReappendsCRLFPerLine(t *testing.T) {
	p, _, err := onePart(t, "a=3Db=\r\nc\r\n--B\r\n", "Content-Transfer-Encoding: quoted-printable\r\n\r\n", "--B", "--B--")
	// Soft break "=\r\n" joins the two lines; "=3D" decodes to "=".
	if got := mustBody(t, p, err); got != "a=bc\r\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestTextPartLatin1BytesAreExpandedToUTF8(t *testing.T) {
	// A raw 0xE9 byte is invalid UTF-8; the Latin-1 fallback expands it to
	// U+00E9 → 0xC3 0xA9. 8bit passes the accumulation through unchanged.
	p, _, err := onePart(t, "caf\xe9\r\n--B--\r\n", "Content-Transfer-Encoding: 8bit\r\n\r\n", "--B", "--B--")
	if got := mustBody(t, p, err); got != "caf\xc3\xa9" {
		t.Fatalf("body = %q", got)
	}
}

func TestBase64DecodesPaddingIndifferentlyAndStripsWhitespace(t *testing.T) {
	p, _, err := onePart(t, "MTIz YWJj\r\n--B--\r\n", "Content-Transfer-Encoding: base64\r\n\r\n", "--B", "--B--")
	if got := mustBody(t, p, err); got != "123abc" {
		t.Fatalf("body = %q", got)
	}
}

func TestBase64InvalidContentIsError(t *testing.T) {
	p, _, err := onePart(t, "!!!!\r\n--B--\r\n", "Content-Transfer-Encoding: base64\r\n\r\n", "--B", "--B--")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Body(); !errors.Is(err, ErrInvalidBase64) {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeForgivingBase64(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string // "!" means an error
	}{
		{"", ""},
		{"QQ==", "A"},
		{"QQ=", "A"},
		{"QQ", "A"},
		{"QUI=", "AB"},
		{"QUI", "AB"},
		{"QUJD", "ABC"},
		{" Q\tU\nJ\fD\r", "ABC"},
		{"QUI==", "!"},  // excess padding
		{"QUJD=", "!"},  // padding on a full quantum
		{"Q", "!"},      // impossible length
		{"Q===", "!"},   // padding after one symbol
		{"====", "!"},   // padding only
		{"QQ=Q", "!"},   // data after padding
		{"QR==", "!"},   // non-zero trailing bits
		{"Q\vQ==", "!"}, // VT is not ASCII whitespace here
	} {
		got, err := decodeForgivingBase64([]byte(tc.in))
		switch {
		case tc.want == "!" && err == nil:
			t.Errorf("decode(%q) = %q, want error", tc.in, got)
		case tc.want != "!" && (err != nil || string(got) != tc.want):
			t.Errorf("decode(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestNonMultipartTextReadsToEOFSuccessfully(t *testing.T) {
	// IE-style single part: no boundary, EOF ends the part.
	p, _, err := onePart(t, "line1\r\nline2\r\n", sevenHdr, "", "")
	if got := mustBody(t, p, err); got != "line1line2" {
		t.Fatalf("body = %q", got)
	}
}

func TestMixedCaseMultipartAlternativeIsNotFlattened(t *testing.T) {
	// The multipart/alternative check is case-SENSITIVE, so a mixed-case
	// multipart/Alternative part is read as one opaque (binary) part whose
	// content type keeps its original case.
	data := "Content-Type: multipart/related; boundary=OUTER\r\n\r\n" +
		"--OUTER\r\n" +
		"Content-Type: multipart/Alternative; boundary=INNER\r\n" +
		"\r\n" +
		"--INNER\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Location: http://x/inner\r\n" +
		"\r\n" +
		"inner\r\n" +
		"--INNER--\r\n" +
		"--OUTER--\r\n"
	a, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	var parts []*Part
	for p, err := range a.Parts() {
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, p)
	}
	if len(parts) != 1 || parts[0].ContentType != "multipart/Alternative" || parts[0].ContentLocation != nil {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestSkipLinesUntilStopsAfterConsumingBoundary(t *testing.T) {
	s := newScanner([]byte("junk\r\npreamble\r\n--B\r\nafter\r\n"), crlf)
	skipLinesUntil(s, []byte("--B"))
	if c, _ := s.nextChunk(); string(c) != "after" {
		t.Fatalf("next = %q", c)
	}
}
