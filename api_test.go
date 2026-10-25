// Public-API behaviours beyond the golden suite: the IE-style non-multipart
// root, multipart/alternative nesting/flattening, the
// Snapshot-Content-Location accessor, and the lenient iterator's
// terminal-error contract.
package mhtml_test

import (
	"testing"

	mhtml "github.com/ajitsoni05/mhtml-go"
)

func TestNonMultipartRootIsASinglePart(t *testing.T) {
	// IE saves a resourceless page as a plain (non-multipart) MIME message:
	// the whole body is one part described by the root header itself.
	data := mhtmlBytes(
		"Content-Type: text/html\r\n",
		"Content-Location: http://example.com/page\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"\r\n",
		"Hello=20World\r\n",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "Hello World\r\n")
	if parts[0].ContentType != "text/html" {
		t.Errorf("ContentType = %q", parts[0].ContentType)
	}
	assertOpt(t, "ContentLocation", parts[0].ContentLocation, "http://example.com/page")
	assertNone(t, "Charset", parts[0].Charset)
}

func TestMultipartAlternativeNestingIsFlattened(t *testing.T) {
	// IE nesting: a multipart/alternative sub-document whose parts are
	// flattened into the single top-level stream, then parsing resumes after
	// the parent boundary.
	data := mhtmlBytes(
		"From: <Saved by Blink>\r\n",
		"Content-Type: multipart/related;\r\n",
		"\tboundary=\"OUTER\"\r\n",
		"\r\n",
		"\r\n",
		"--OUTER\r\n",
		"Content-Type: multipart/alternative;\r\n",
		"\tboundary=\"INNER\"\r\n",
		"\r\n",
		"inner-preamble\r\n",
		"--INNER\r\n",
		"Content-Location: http://example.com/a\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/html\r\n",
		"\r\n",
		"AAA\r\n",
		"--INNER\r\n",
		"Content-Location: http://example.com/b\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/plain\r\n",
		"\r\n",
		"BBB\r\n",
		"--INNER--\r\n",
		"--OUTER\r\n",
		"Content-Location: http://example.com/c\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/css\r\n",
		"\r\n",
		"CCC\r\n",
		"--OUTER--\r\n",
		"\x00",
	)

	_, parts := parseAll(t, data)
	assertBodies(t, parts, "AAA", "BBB", "CCC")
	for i, want := range []string{"http://example.com/a", "http://example.com/b", "http://example.com/c"} {
		assertOpt(t, "ContentLocation", parts[i].ContentLocation, want)
	}
}

func TestSnapshotContentLocationIsExposedFromRoot(t *testing.T) {
	data := mhtmlBytes(
		"From: <Saved by Blink>\r\n",
		"Snapshot-Content-Location: http://example.com/saved-page\r\n",
		"Content-Type: multipart/related;\r\n",
		"\tboundary=\"BoUnDaRy\"\r\n",
		"\r\n",
		"\r\n",
		"--BoUnDaRy\r\n",
		"Content-Location: http://example.com/page1\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/html\r\n",
		"\r\n",
		"body\r\n",
		"--BoUnDaRy--\r\n",
		"\x00",
	)

	archive, err := mhtml.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if loc, ok := archive.SnapshotContentLocation(); !ok || loc != "http://example.com/saved-page" {
		t.Fatalf("SnapshotContentLocation = %q, %v", loc, ok)
	}
}

func TestSnapshotContentLocationIsNoneWhenAbsent(t *testing.T) {
	data := mhtmlBytes(
		"Content-Type: multipart/related; boundary=\"B\"\r\n",
		"\r\n",
		"\r\n",
		"--B\r\n",
		"Content-Transfer-Encoding: 7bit\r\n",
		"Content-Type: text/html\r\n",
		"\r\n",
		"x\r\n",
		"--B--\r\n",
		"\x00",
	)

	archive, err := mhtml.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if loc, ok := archive.SnapshotContentLocation(); ok {
		t.Fatalf("SnapshotContentLocation = %q, want none", loc)
	}
}

// corruptArchive's second part is never terminated by a boundary (EOF first).
var corruptArchive = mhtmlBytes(
	"Content-Type: multipart/related; boundary=\"B\"\r\n",
	"\r\n",
	"\r\n",
	"--B\r\n",
	"Content-Transfer-Encoding: 7bit\r\n",
	"Content-Type: text/plain\r\n",
	"\r\n",
	"good\r\n",
	"--B\r\n",
	"Content-Transfer-Encoding: 7bit\r\n",
	"Content-Type: text/plain\r\n",
	"\r\n",
	"no closing boundary here",
)

func TestIteratorYieldsPartsUntilCorruptionThenOneError(t *testing.T) {
	// The iterator yields the good part, then exactly one error, then stops.
	// Already-yielded parts stand.
	archive, err := mhtml.Parse(corruptArchive)
	if err != nil {
		t.Fatal(err)
	}
	var parts []*mhtml.Part
	var errs []error
	for part, err := range archive.Parts() {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) != 1 || len(errs) != 1 {
		t.Fatalf("got %d parts and %d errors, want 1 and 1", len(parts), len(errs))
	}
	if parts[0].TransferEncoding != mhtml.SevenBit {
		t.Errorf("TransferEncoding = %v", parts[0].TransferEncoding)
	}
	if body, err := parts[0].Body(); err != nil || string(body) != "good" {
		t.Errorf("Body = %q, %v", body, err)
	}
}

func TestParseAllIsAllOrNothingOnCorruption(t *testing.T) {
	archive, err := mhtml.Parse(corruptArchive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.ParseAll(); err == nil {
		t.Fatal("ParseAll succeeded, want error")
	}
}
