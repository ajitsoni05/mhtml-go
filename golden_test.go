// Golden acceptance suite: a faithful port of the Rust crate's golden suite
// (itself a port of the reference parser's tests), plus the
// content-ID-to-URI conversion cases.
//
// Fidelity rules honoured here:
//   - Each reference test input is reproduced byte-for-byte (pieces
//     concatenate with NO separator, so several parts are deliberately joined
//     without a CRLF). The shared root header is factored into blinkPreamble
//     and dateArchive; the bytes are unchanged.
//   - The reference tests parse a NUL-terminated C string, so the trailing NUL
//     is part of the parsed input — each input therefore ends with "\x00".
//
// Documented divergences from the reference (all golden cases still agree):
//   - Failure is reported as an error, not the reference's empty resource
//     vector (see TestMissingBoundary).
//   - Date parsing accepts RFC 2822 only; the reference parser is looser.
package mhtml_test

import (
	"bytes"
	"testing"
	"time"

	mhtml "github.com/ajitsoni05/mhtml-go"
)

// mhtmlBytes concatenates pieces into one input, preserving every byte.
func mhtmlBytes(pieces ...string) []byte {
	var buf bytes.Buffer
	for _, p := range pieces {
		buf.WriteString(p)
	}
	return buf.Bytes()
}

func parseAll(t *testing.T, data []byte) (*mhtml.Archive, []mhtml.DecodedPart) {
	t.Helper()
	archive, err := mhtml.Parse(data)
	if err != nil {
		t.Fatalf("archive should parse: %v", err)
	}
	parts, err := archive.ParseAll()
	if err != nil {
		t.Fatalf("all parts should decode: %v", err)
	}
	return archive, parts
}

func assertOpt(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %q", name, fmtOpt(got), want)
	}
}

func assertNone(t *testing.T, name string, got *string) {
	t.Helper()
	if got != nil {
		t.Errorf("%s = %q, want nil", name, *got)
	}
}

func fmtOpt(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func assertBodies(t *testing.T, parts []mhtml.DecodedPart, want ...string) {
	t.Helper()
	if len(parts) != len(want) {
		t.Fatalf("got %d parts, want %d", len(parts), len(want))
	}
	for i, p := range parts {
		if string(p.Body) != want[i] {
			t.Errorf("parts[%d].Body = %q, want %q", i, p.Body, want[i])
		}
	}
}

// blinkPreamble is the root header every reference test shares, through the
// boundary's two trailing CRLFs.
const blinkPreamble = "From: <Saved by Blink>\r\n" +
	"Subject: Test Subject\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/related;\r\n" +
	"\ttype=\"text/html\";\r\n" +
	"\tboundary=\"BoUnDaRy\"\r\n" +
	"\r\n" +
	"\r\n"

func TestMHTMLPartHeaders(t *testing.T) {
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"single line\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-ID: <foo-123@mhtml.blink>\r\n",
		"Content-Transfer-Encoding: binary\r\n",
		"Content-Type: text/plain\r\n",
		"\r\n",
		"bin\x00ary\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page3\r\n",
		"Content-Transfer-Encoding: base64\r\n",
		"Content-Type: text/css; charset=ascii\r\n",
		"\r\n",
		"MTIzYWJj\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(parts))
	}

	assertOpt(t, "parts[0].ContentLocation", parts[0].ContentLocation, "http://www.example.com/page1")
	assertNone(t, "parts[0].ContentID", parts[0].ContentID)
	if parts[0].ContentType != "text/html" {
		t.Errorf("parts[0].ContentType = %q", parts[0].ContentType)
	}
	assertOpt(t, "parts[0].Charset", parts[0].Charset, "utf-8")

	assertOpt(t, "parts[1].ContentLocation", parts[1].ContentLocation, "http://www.example.com/page2")
	assertOpt(t, "parts[1].ContentID", parts[1].ContentID, "<foo-123@mhtml.blink>")
	if parts[1].ContentType != "text/plain" {
		t.Errorf("parts[1].ContentType = %q", parts[1].ContentType)
	}
	assertNone(t, "parts[1].Charset", parts[1].Charset)

	assertOpt(t, "parts[2].ContentLocation", parts[2].ContentLocation, "http://www.example.com/page3")
	assertNone(t, "parts[2].ContentID", parts[2].ContentID)
	if parts[2].ContentType != "text/css" {
		t.Errorf("parts[2].ContentType = %q", parts[2].ContentType)
	}
	assertOpt(t, "parts[2].Charset", parts[2].Charset, "ascii")
}

func TestQuotedPrintableContentTransferEncoding(t *testing.T) {
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"single line\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"Content-Type: text/plain\r\n",
		"\r\n",
		"long line=3Dbar=3D=\r\n",
		"more\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page3\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"Content-Type: text/css; charset=ascii\r\n",
		"\r\n",
		"first line\r\n",
		"second line\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts,
		"single line\r\n",
		"long line=bar=more\r\n",
		"first line\r\nsecond line\r\n\r\n",
	)
}

func TestBase64ContentTransferEncoding(t *testing.T) {
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Transfer-Encoding: base64\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"MTIzYWJj\r\n",
		"\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: base64\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"MTIzYWJj\r\n",
		"AQIDDQ4P\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "123abc", "123abc\x01\x02\x03\x0d\x0e\x0f")
}

func TestEightBitContentTransferEncoding(t *testing.T) {
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: 8bit\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"123\r\n",
		"bin\x00ary\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "123bin\x00ary")
}

func TestSevenBitContentTransferEncoding(t *testing.T) {
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"123\r\n",
		"abcdefg\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "123abcdefg")
}

func TestSpaceAsHeaderContinuation(t *testing.T) {
	// The boundary parameter is folded onto a continuation line that begins
	// with a single space (rather than a tab) — still a valid header fold.
	data := mhtmlBytes(
		"From: <Saved by Blink>\r\n",
		"Subject: Test Subject\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: multipart/related;\r\n",
		"\ttype=\"text/html\";\r\n",
		" boundary=\"BoUnDaRy\"\r\n",
		"\r\n",
		"\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/html; charset=utf-8\r\n",
		"\r\n",
		"123\r\n",
		"abcdefg\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "123abcdefg")
}

func TestBinaryContentTransferEncoding(t *testing.T) {
	// Part 2's body ("bin\0ary") is joined to the next boundary with NO CRLF.
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Transfer-Encoding: binary\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"bin\x00ary\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: binary\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"bin\x00ary",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page3\r\n",
		"Content-Transfer-Encoding: binary\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "bin\x00ary", "bin\x00ary", "")
}

func TestUnknownContentTransferEncoding(t *testing.T) {
	// Unknown encoding is treated as binary.
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Transfer-Encoding: foo\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"bin\x00ary\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Transfer-Encoding: unknown\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"bin\x00ary",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page3\r\n",
		"Content-Transfer-Encoding: \r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "bin\x00ary", "bin\x00ary", "")
}

func TestNoContentTransferEncoding(t *testing.T) {
	// Missing encoding is treated as binary.
	data := mhtmlBytes(
		blinkPreamble,
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"bin\x00ary\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page2\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"bin\x00ary",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page3\r\n",
		"Content-Type: binary/octet-stream\r\n",
		"\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "bin\x00ary", "bin\x00ary", "")
}

// dateArchive is the reference date-test archive with dateLine (a full
// header line, or "") inserted after the Subject header.
func dateArchive(dateLine, partDateLine string) []byte {
	return mhtmlBytes(
		"From: <Saved by Blink>\r\n",
		"Subject: Test Subject\r\n",
		dateLine,
		"MIME-Version: 1.0\r\n",
		"Content-Type: multipart/related;\r\n",
		"\ttype=\"text/html\";\r\n",
		"\tboundary=\"BoUnDaRy\"\r\n",
		"\r\n",
		"\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://www.example.com/page1\r\n",
		"Content-Type: binary/octet-stream\r\n",
		partDateLine,
		"\r\n",
		"bin\x00ary\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)
}

func assertNoCreationDate(t *testing.T, data []byte) {
	t.Helper()
	archive, parts := parseAll(t, data)
	if len(parts) == 0 {
		t.Fatal("no parts")
	}
	if d, ok := archive.CreationDate(); ok {
		t.Fatalf("CreationDate = %v, want none", d)
	}
}

func TestDateParsingEmptyDate(t *testing.T) {
	// Missing date is ignored.
	assertNoCreationDate(t, dateArchive("", ""))
}

func TestDateParsingInvalidDate(t *testing.T) {
	// Invalid archive date is ignored. Also, a Date header *within a part*
	// must not be used as the archive creation date.
	assertNoCreationDate(t, dateArchive("Date: 123xyz\r\n", "Date: Fri, 1 Mar 2017 22:44:17 -0000\r\n"))
}

func TestDateParsingValidDate(t *testing.T) {
	// Valid archive-level date is used: Fri, 1 Mar 2017 22:44:17 UTC.
	archive, parts := parseAll(t, dateArchive("Date: Fri, 1 Mar 2017 22:44:17 -0000\r\n", ""))
	if len(parts) == 0 {
		t.Fatal("no parts")
	}
	// 2017-03-01 22:44:17 UTC == unix 1_488_408_257.
	if d, ok := archive.CreationDate(); !ok || !d.Equal(time.Unix(1_488_408_257, 0)) {
		t.Fatalf("CreationDate = %v, %v", d, ok)
	}
}

func TestMissingBoundary(t *testing.T) {
	// No boundary parameter in a multipart content type: the reference returns
	// an empty resource vector. DIVERGENCE: we surface this as an error.
	if _, err := mhtml.Parse(mhtmlBytes("Content-Type: multipart/false\r\n", "\x00")); err == nil {
		t.Fatal("Parse succeeded, want error")
	}
}

func TestOverflowedDate(t *testing.T) {
	// A malformed/overflowing Date must be ignored (no creation date).
	assertNoCreationDate(t, dateArchive("Date:May1 922372\r\n", ""))
}

func TestOverflowedDay(t *testing.T) {
	// A malformed/overflowing Date day must be ignored (no creation date).
	assertNoCreationDate(t, dateArchive("Date:94/3/933720368547\r\n", ""))
}

func TestContentIDToCIDURLConversions(t *testing.T) {
	// "<foo@bar.com>" -> "cid:foo@bar.com".
	if got, ok := mhtml.ContentIDToCIDURL("<foo@bar.com>"); !ok || got != "cid:foo@bar.com" {
		t.Errorf("ContentIDToCIDURL(<foo@bar.com>) = %q, %v", got, ok)
	}
	for _, id := range []string{
		"", "<>", // length <= 2
		"foo@bar", "<foo", // not wrapped in <...>
	} {
		if got, ok := mhtml.ContentIDToCIDURL(id); ok {
			t.Errorf("ContentIDToCIDURL(%q) = %q, want none", id, got)
		}
	}
}
