package mhtml

import (
	"errors"
	"maps"
	"testing"
	"time"
)

func kv(input string) map[string]string {
	return retrieveKeyValuePairs(newScanner([]byte(input), crlf))
}

func assertPairs(t *testing.T, got, want map[string]string) {
	t.Helper()
	if !maps.Equal(got, want) {
		t.Fatalf("pairs = %q, want %q", got, want)
	}
}

func TestFoldsContinuationLinesWithSpaceAndTab(t *testing.T) {
	// Both a space- and a tab-prefixed continuation append, stripping only the
	// single leading whitespace byte.
	m := kv("Content-Type: multipart/related;\r\n boundary=abc\r\n\ttail\r\n\r\n")
	assertPairs(t, m, map[string]string{"content-type": "multipart/related;boundary=abctail"})
}

func TestDuplicateKeyLastValueWins(t *testing.T) {
	assertPairs(t, kv("X: first\r\nX: second\r\n\r\n"), map[string]string{"x": "second"})
}

func TestDuplicateNonFinalKeyKeepsFirstValue(t *testing.T) {
	// Non-final headers are flushed via a non-overwriting insert (only the
	// trailing header overwrites), so a key duplicated *before* the final
	// header is first-wins.
	m := kv("Content-Type: text/plain\r\n" +
		"Content-Type: text/html\r\n" +
		"Content-Location: http://x/\r\n\r\n")
	assertPairs(t, m, map[string]string{"content-type": "text/plain", "content-location": "http://x/"})
}

func TestValueTrimStripsOnlyASCIIWhitespaceNotNBSP(t *testing.T) {
	// A trailing 0xA0 byte (Latin-1-decoded to U+00A0 NBSP) is preserved;
	// strings.TrimSpace would wrongly drop it.
	m := kv("Content-Location: http://x/\xa0\r\n\r\n")
	assertPairs(t, m, map[string]string{"content-location": "http://x/ "})
}

func TestLineWithoutColonIsIgnoredButFlushesPendingKey(t *testing.T) {
	m := kv("Content-ID: <id@host>\r\nnonsense line\r\nContent-Type: text/html\r\n\r\n")
	assertPairs(t, m, map[string]string{"content-id": "<id@host>", "content-type": "text/html"})
}

func TestEmptyLineEndsSectionAndKeyIsLowercasedTrimmed(t *testing.T) {
	// The value after the empty divider line is not read into the map.
	assertPairs(t, kv("KEY  : value\r\n\r\nnot: read\r\n"), map[string]string{"key": "value"})
}

func TestNonASCIIKeyIsNotCaseFolded(t *testing.T) {
	// Only A-Z are lowercased: the Latin-1 "É" (0xC9) survives as-is.
	assertPairs(t, kv("X\xc9: v\r\n\r\n"), map[string]string{"xÉ": "v"})
}

func parseHeader(t *testing.T, input string) *mimeHeader {
	t.Helper()
	h, err := parseMimeHeader(newScanner([]byte(input), crlf))
	if err != nil {
		t.Fatalf("header parse: %v", err)
	}
	return h
}

func assertOpt(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %q", name, fmtOpt(got), want)
	}
}

func assertNone(t *testing.T, name string, got *string) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %q, want nil", name, *got)
	}
}

func fmtOpt(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestMultipartQuotedBoundaryBuildsSentinels(t *testing.T) {
	h := parseHeader(t, "Content-Type: multipart/related; type=\"text/html\"; boundary=\"BND\"\r\n\r\n")
	if h.contentType != "multipart/related" || h.partBoundary != "--BND" || h.docBoundary != "--BND--" {
		t.Fatalf("header = %+v", h)
	}
	// Multipart headers carry no charset.
	assertNone(t, "charset", h.charset)
}

func TestMultipartUnquotedBoundaryBuildsSentinels(t *testing.T) {
	h := parseHeader(t, "Content-Type: multipart/related; boundary=--xyz\r\n\r\n")
	if h.partBoundary != "----xyz" || h.docBoundary != "----xyz--" {
		t.Fatalf("boundaries = %q, %q", h.partBoundary, h.docBoundary)
	}
}

func TestMultipartMissingBoundaryIsError(t *testing.T) {
	_, err := parseMimeHeader(newScanner([]byte("Content-Type: multipart/related\r\n\r\n"), crlf))
	if !errors.Is(err, ErrMissingBoundary) {
		t.Fatalf("err = %v, want ErrMissingBoundary", err)
	}
}

func TestNonMultipartExtractsCharsetAndFields(t *testing.T) {
	h := parseHeader(t, "Content-Type: text/html; charset=\"UTF-8\"\r\n"+
		"Content-Transfer-Encoding: quoted-printable\r\n"+
		"Content-Location: http://example.com/\r\n"+
		"Content-ID: <id@host>\r\n\r\n")
	if h.contentType != "text/html" || h.transferEncoding != QuotedPrintable || h.partBoundary != "" {
		t.Fatalf("header = %+v", h)
	}
	assertOpt(t, "charset", h.charset, "UTF-8")
	assertOpt(t, "content-location", h.contentLocation, "http://example.com/")
	assertOpt(t, "content-id", h.contentID, "<id@host>")
}

func TestAbsentContentTypeYieldsEmptyNonMultipartHeader(t *testing.T) {
	h := parseHeader(t, "Content-Location: http://x/\r\n\r\n")
	if h.contentType != "" || h.isMultipart() || h.transferEncoding != Unknown {
		t.Fatalf("header = %+v", h)
	}
}

func TestMixedCaseMultipartPrefixIsMultipart(t *testing.T) {
	h := parseHeader(t, "Content-Type: MultiPart/Related; boundary=B\r\n\r\n")
	if !h.isMultipart() || h.contentType != "MultiPart/Related" {
		t.Fatalf("header = %+v", h)
	}
}

func TestSnapshotContentLocationIsExtractedFromRootHeader(t *testing.T) {
	h := parseHeader(t, "Snapshot-Content-Location: http://example.com/page\r\n"+
		"Content-Type: multipart/related; boundary=\"B\"\r\n\r\n")
	assertOpt(t, "snapshot-content-location", h.snapshotContentLocation, "http://example.com/page")
}

func TestDateHeaderParsedIntoCreationTime(t *testing.T) {
	h := parseHeader(t, "Date: Fri, 1 Mar 2017 22:44:17 -0000\r\n\r\n")
	if h.date == nil || !h.date.Equal(time.Unix(1_488_408_257, 0)) {
		t.Fatalf("date = %v", h.date)
	}
}

func TestContentTypeMIMEIsTrimmedAndCasePreserved(t *testing.T) {
	// Case-insensitivity is applied separately at comparison sites.
	if p := parseContentType("  TEXT/HTML ; charset=utf-8"); p.mimeType != "TEXT/HTML" {
		t.Fatalf("mime = %q", p.mimeType)
	}
}

func TestContentTypeQuotedAndUnquotedParamValues(t *testing.T) {
	for _, v := range []string{
		"multipart/related; boundary=\"----=_Part_0\"",
		"multipart/related; boundary=----=_Part_0",
	} {
		if b, ok := parseContentType(v).param("boundary"); !ok || b != "----=_Part_0" {
			t.Fatalf("%q: boundary = %q, %v", v, b, ok)
		}
	}
}

func TestContentTypeSkipsValuelessAndEmptySegments(t *testing.T) {
	p := parseContentType("text/html; ; junk; charset=utf-8;")
	if _, ok := p.param("junk"); ok {
		t.Fatal("valueless segment became a param")
	}
	if c, _ := p.param("charset"); c != "utf-8" {
		t.Fatalf("charset = %q", c)
	}
}

func TestEncodingTokensMapToVariants(t *testing.T) {
	for in, want := range map[string]TransferEncoding{
		"base64":                Base64,
		"  Quoted-Printable \t": QuotedPrintable,
		"8BIT":                  EightBit,
		"7bit":                  SevenBit,
		"binary":                Binary,
		"uuencode":              Unknown,
		"":                      Unknown,
	} {
		if got := parseContentTransferEncoding(in); got != want {
			t.Errorf("parseContentTransferEncoding(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseDate(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64 // unix seconds; -1 means rejected
	}{
		// One-digit day and the -0000 "unknown local zone" form.
		{"Fri, 1 Mar 2017 22:44:17 -0000", 1_488_408_257},
		{"123xyz", -1},
		{"May1 922372", -1},
		{"94/3/933720368547", -1},
		// Two-digit years pivot at 50, not net/mail's 69.
		{"1 Jan 49 00:00:00 +0000", 2493072000},
		{"1 Jan 50 00:00:00 +0000", -631152000},
		{"1 Jan 1899 00:00:00 +0000", -1},
		// Seconds are optional; comments are CFWS, except after a 4-digit
		// year, which takes plain FWS.
		{"1 Mar 2017 22:44 +0000", 1_488_408_240},
		{"(c) Wed, 1 (x (y)) Mar 2017 22:44:17 (z) +0000 (UTC)", 1_488_408_257},
		{"1 Mar 2017 (x) 22:44:17 +0000", -1},
		// Obsolete zones: named, GMT, trailing UT, military letters.
		{"1 Mar 2017 22:44:17 EST", 1_488_408_257 + 5*3600},
		{"1 Mar 2017 22:44:17 pdt", 1_488_408_257 + 7*3600},
		{"1 Mar 2017 22:44:17 GMT", 1_488_408_257},
		{"1 Mar 2017 22:44:17 UT", 1_488_408_257},
		{"1 Mar 2017 22:44:17 Z", 1_488_408_257},
		{"1 Mar 2017 22:44:17 J", -1},
		{"1 Mar 2017 22:44:17 UT (c)", -1},
		{"1 Mar 2017 22:44:17 XYZ", -1},
		// The minute's sign follows the hour's: -0030 is +00:30.
		{"1 Mar 2017 22:44:17 -0030", 1_488_408_257 - 1800},
		{"1 Mar 2017 22:44:17 -0130", 1_488_408_257 + 5400},
		{"1 Mar 2017 22:44:17 +2600", -1},
		{"1 Mar 2017 22:44:17 +0060", -1},
		// Calendar and clock validation.
		{"29 Feb 2016 00:00:00 +0000", 1456704000},
		{"29 Feb 2017 00:00:00 +0000", -1},
		{"1 Mar 2017 24:00:00 +0000", -1},
		// A leap second is only a stand-in for a month's last UTC second.
		{"31 Dec 2016 23:59:60 +0000", 1483228799},
		{"30 Dec 2016 23:59:60 +0000", -1},
		{"1 Jan 2017 00:59:60 +0100", 1483228799},
		// The weekday needs its comma, and nothing may trail the zone.
		{"Fri 1 Mar 2017 22:44:17 +0000", -1},
		{"1 Mar 2017 22:44:17 +0000 x", -1},
		{" \t1 Mar 2017 22:44:17 +0000 ", 1_488_408_257},
	} {
		got := parseDate(tc.in)
		switch {
		case tc.want == -1 && got != nil:
			t.Errorf("parseDate(%q) = %v, want rejected", tc.in, got)
		case tc.want != -1 && (got == nil || got.Unix() != tc.want):
			t.Errorf("parseDate(%q) = %v, want unix %d", tc.in, got, tc.want)
		}
	}
}

func TestParseDateNestedCommentsAreDepthLimited(t *testing.T) {
	deep := ""
	for range 40 {
		deep = "(" + deep + ")"
	}
	if got := parseDate("1 Mar 2017 22:44:17 +0000 " + deep); got != nil {
		t.Fatalf("parseDate accepted %d-deep comments: %v", 40, got)
	}
}
