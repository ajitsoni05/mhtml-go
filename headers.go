package mhtml

import (
	"strings"
	"time"
)

// stripASCIIWhitespace trims leading/trailing ASCII whitespace only (TAB, LF,
// FF, CR, SPACE). strings.TrimSpace additionally strips Unicode White_Space
// (e.g. NBSP U+00A0), which the Latin-1 header fallback can introduce; using
// this preserves such bytes exactly.
func stripASCIIWhitespace(s string) string {
	return strings.Trim(s, "\t\n\f\r ")
}

// asciiLower lowercases A-Z only; strings.ToLower would also fold the
// non-ASCII letters the Latin-1 fallback can produce.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// parseContentTransferEncoding maps a Content-Transfer-Encoding value to its
// TransferEncoding. The value is whitespace-trimmed and ASCII-lowercased; any
// token outside the known set (including an empty value) becomes Unknown.
func parseContentTransferEncoding(value string) TransferEncoding {
	switch asciiLower(stripASCIIWhitespace(value)) {
	case "base64":
		return Base64
	case "quoted-printable":
		return QuotedPrintable
	case "8bit":
		return EightBit
	case "7bit":
		return SevenBit
	case "binary":
		return Binary
	}
	return Unknown
}

// retrieveKeyValuePairs reads the RFC 822 key/value section from s (which
// must be set to the CRLF separator).
//
// Stops at EOF or the first empty line (the header/body divider, consumed).
// Lines starting with a space or tab continue the previous value, appending
// everything after that one leading whitespace byte. Otherwise the pending key
// is flushed (values trimmed on both ends) and the line is split at its first
// ':' into a trimmed+lowercased key and a verbatim value; a line with no ':'
// is ignored (but still flushes the pending key). Duplicate keys: an
// intermediate occurrence is inserted only when absent (first-wins), but the
// trailing key overwrites (last-wins).
func retrieveKeyValuePairs(s *scanner) map[string]string {
	pairs := map[string]string{}
	var key string
	haveKey := false
	var value strings.Builder
	for chunk, ok := s.nextChunk(); ok; chunk, ok = s.nextChunk() {
		line := decodeUTF8Latin1(chunk)
		if line == "" {
			break
		}
		if line[0] == '\t' || line[0] == ' ' {
			value.WriteString(line[1:])
			continue
		}
		if haveKey {
			// Only the final insert below overwrites, so a key duplicated
			// before the final header is first-wins.
			if _, exists := pairs[key]; !exists {
				pairs[key] = stripASCIIWhitespace(value.String())
			}
			value.Reset()
			haveKey = false
		}
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key = asciiLower(stripASCIIWhitespace(line[:idx]))
		haveKey = true
		value.WriteString(line[idx+1:])
	}
	if haveKey {
		pairs[key] = stripASCIIWhitespace(value.String())
	}
	return pairs
}

// mimeHeader is the parsed MIME header of one MHTML part (or the archive
// root).
type mimeHeader struct {
	// MIME type in its original case ("" when no Content-Type header was
	// present); callers compare it case-insensitively where appropriate.
	contentType string
	// charset parameter of a non-multipart Content-Type.
	charset          *string
	transferEncoding TransferEncoding
	contentLocation  *string
	contentID        *string
	date             *time.Time
	// Archive-level Snapshot-Content-Location header (the saved page's URL).
	// Only meaningful on the root header; ignored on part headers.
	snapshotContentLocation *string
	// "--" + boundary, delimiting parts (multipart only).
	partBoundary string
	// "--" + boundary + "--", delimiting the archive (multipart only).
	docBoundary string
}

// parseMimeHeader reads and interprets a part's MIME header from s (set to
// CRLF). It errors only when a multipart header lacks a boundary parameter.
func parseMimeHeader(s *scanner) (*mimeHeader, error) {
	pairs := retrieveKeyValuePairs(s)
	h := &mimeHeader{transferEncoding: Unknown}

	if contentType, ok := pairs["content-type"]; ok {
		parsed := parseContentType(contentType)
		h.contentType = parsed.mimeType
		if h.isMultipart() {
			boundary, ok := parsed.param("boundary")
			if !ok {
				return nil, ErrMissingBoundary
			}
			h.partBoundary = "--" + boundary
			h.docBoundary = "--" + boundary + "--"
		} else if charset, ok := parsed.param("charset"); ok {
			charset = stripASCIIWhitespace(charset)
			h.charset = &charset
		}
	}

	if encoding, ok := pairs["content-transfer-encoding"]; ok {
		h.transferEncoding = parseContentTransferEncoding(encoding)
	}
	h.contentLocation = lookup(pairs, "content-location")
	h.contentID = lookup(pairs, "content-id")
	h.snapshotContentLocation = lookup(pairs, "snapshot-content-location")
	if date, ok := pairs["date"]; ok {
		h.date = parseDate(date)
	}
	return h, nil
}

func lookup(pairs map[string]string, key string) *string {
	if v, ok := pairs[key]; ok {
		return &v
	}
	return nil
}

// isMultipart reports whether this is a multipart/* container (ASCII
// case-insensitive; contentType preserves original case).
func (h *mimeHeader) isMultipart() bool {
	const prefix = "multipart/"
	if len(h.contentType) < len(prefix) {
		return false
	}
	return asciiLower(h.contentType[:len(prefix)]) == prefix
}

// contentType is a Content-Type value split into its (original-case) MIME
// type and parameter list, parsed in relaxed mode.
type contentType struct {
	mimeType string
	params   [][2]string
}

// parseContentType parses value leniently: the MIME type is everything before
// the first ';' (ASCII-whitespace-trimmed; original case preserved);
// parameters are ';'-separated name=value pairs with trimmed+lowercased names
// and trimmed values (one surrounding pair of double quotes stripped).
// Segments without '=', and empty segments, are skipped.
//
// Unlike the header trims, these use strings.TrimSpace (Unicode White_Space).
func parseContentType(value string) contentType {
	segments := strings.Split(value, ";")
	ct := contentType{mimeType: stripASCIIWhitespace(segments[0])}
	for _, segment := range segments[1:] {
		if strings.TrimSpace(segment) == "" {
			continue
		}
		name, val, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		name = asciiLower(strings.TrimSpace(name))
		ct.params = append(ct.params, [2]string{name, unquote(strings.TrimSpace(val))})
	}
	return ct
}

// param returns the value of parameter name (already lowercased), if present.
func (ct contentType) param(name string) (string, bool) {
	for _, p := range ct.params {
		if p[0] == name {
			return p[1], true
		}
	}
	return "", false
}

// unquote strips a single surrounding pair of double quotes, if present.
func unquote(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1]
	}
	return value
}
