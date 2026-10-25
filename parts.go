package mhtml

import (
	"bytes"
	"encoding/base64"
	"iter"
)

// Part is one archive part; its body is decoded on demand.
type Part struct {
	ContentType     string
	Charset         *string
	ContentLocation *string
	ContentID       *string
	// The *effective* transfer encoding: an Unknown/absent header has already
	// been coerced to Binary (real archives rely on this).
	TransferEncoding TransferEncoding
	// The pre-decode content region: for binary parts a slice of the archive
	// input; for the text encodings the line-joined accumulation.
	raw []byte
}

// Body decodes the part according to its transfer encoding.
//
// Binary/7bit/8bit return their accumulated bytes without copying (binary
// aliases the archive input), so callers must not modify the result.
// Quoted-printable and base64 always allocate; only base64 can fail.
func (p *Part) Body() ([]byte, error) {
	switch p.TransferEncoding {
	case Base64:
		return decodeForgivingBase64(p.raw)
	case QuotedPrintable:
		return decodeQuotedPrintable(p.raw), nil
	}
	return p.raw, nil
}

// decodeForgivingBase64 implements forgiving-base64 (WHATWG Infra), which real
// archives rely on: ASCII whitespace is stripped, and padding may be canonical,
// partial or absent — but never excess. Non-zero trailing bits are rejected.
func decodeForgivingBase64(raw []byte) ([]byte, error) {
	stripped := make([]byte, 0, len(raw))
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			stripped = append(stripped, c)
		}
	}
	data := bytes.TrimRight(stripped, "=")
	if pad := len(stripped) - len(data); pad > 0 {
		r := len(data) % 4
		if !(r == 2 && pad <= 2 || r == 3 && pad == 1) {
			return nil, ErrInvalidBase64
		}
	}
	enc := base64.RawStdEncoding.Strict()
	out := make([]byte, enc.DecodedLen(len(data)))
	n, err := enc.Decode(out, data)
	if err != nil {
		return nil, ErrInvalidBase64
	}
	return out[:n], nil
}

// DecodedPart is a part with its body already decoded (produced by
// [Archive.ParseAll]). As with [Part.Body], Body may alias the archive input.
type DecodedPart struct {
	ContentType      string
	Charset          *string
	ContentLocation  *string
	ContentID        *string
	TransferEncoding TransferEncoding
	Body             []byte
}

// boundaryContext is a multipart/* boundary context: partBoundary ends a
// part, docBoundary ends the (sub)document.
type boundaryContext struct {
	partBoundary, docBoundary []byte
}

func newBoundaryContext(h *mimeHeader) boundaryContext {
	return boundaryContext{[]byte(h.partBoundary), []byte(h.docBoundary)}
}

// Parts lazily iterates the archive's parts. It is lenient: it yields parts
// with a nil error until the first corruption, then that one error, then
// stops.
//
// multipart/alternative nesting (IE style) is flattened into the same stream
// via an explicit stack of boundary contexts.
func (a *Archive) Parts() iter.Seq2[*Part, error] {
	return func(yield func(*Part, error) bool) {
		s := newScanner(a.body, crlf)
		if !a.root.isMultipart() {
			// A non-multipart root is exactly one part described by the root
			// header: nothing follows, whether it decodes or errors.
			part, _, err := parseNextPart(s, a.root, nil, nil)
			yield(part, err)
			return
		}

		// A missing first boundary is not itself an error: the header parse
		// over EOF yields an empty header whose binary path then errors.
		root := newBoundaryContext(a.root)
		skipLinesUntil(s, root.partBoundary)
		stack := []boundaryContext{root}
		for len(stack) > 0 {
			header, err := parseMimeHeader(s)
			if err != nil {
				yield(nil, err)
				return
			}

			// IE nesting: descend into a multipart/alternative sub-document
			// (skipping its preamble) and flatten its parts into this stream.
			// The comparison is case-sensitive against the original-case MIME
			// type; a multipart header always carries both boundaries.
			if header.contentType == "multipart/alternative" {
				nested := newBoundaryContext(header)
				skipLinesUntil(s, nested.partBoundary)
				stack = append(stack, nested)
				continue
			}

			ctx := stack[len(stack)-1]
			part, endOfArchive, err := parseNextPart(s, header, ctx.partBoundary, ctx.docBoundary)
			if err != nil {
				yield(nil, err)
				return
			}
			if endOfArchive {
				// This (sub)document is finished; resume the parent, if any,
				// at its next part boundary.
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					skipLinesUntil(s, stack[len(stack)-1].partBoundary)
				}
			}
			if !yield(part, nil) {
				return
			}
		}
	}
}

// skipLinesUntil consumes CRLF lines until one exactly equals boundary (then
// stops, having consumed it) or EOF is reached; callers ignore whether the
// boundary was found.
func skipLinesUntil(s *scanner, boundary []byte) {
	for chunk, ok := s.nextChunk(); ok; chunk, ok = s.nextChunk() {
		if bytes.Equal(chunk, boundary) {
			return
		}
	}
}

// parseNextPart parses the body region of one part, given its already-parsed
// header and the enclosing context's boundaries (both nil for a non-multipart
// root part). It also reports whether the end-of-document boundary was
// reached.
func parseNextPart(s *scanner, header *mimeHeader, partBoundary, docBoundary []byte) (*Part, bool, error) {
	// If no content transfer encoding is specified, default to binary.
	encoding := header.transferEncoding
	if encoding == Unknown {
		encoding = Binary
	}
	endOfArchive := false
	endOfPartReached := false
	var raw []byte

	if encoding == Binary {
		if partBoundary == nil {
			// Binary contents require an end-of-part boundary to delimit them.
			return nil, false, ErrBinaryPartWithoutBoundary
		}
		// Bug-compat: real writers do not prepend CRLF to the boundary after
		// a binary part, so we split on the *raw* boundary (no leading CRLF).
		// The content may therefore keep a trailing CRLF, stripped below.
		s.sep = partBoundary
		chunk, ok := s.nextChunk()
		if !ok {
			// EOF without ever reaching the boundary.
			return nil, false, ErrUnterminatedPart
		}
		s.sep = crlf

		// Strip one trailing CRLF if present (it may really belong to the
		// content, but this is the bug-compat risk accepted here).
		raw = bytes.TrimSuffix(chunk, crlf)
		raw = raw[:len(raw):len(raw)]

		next2 := s.peek(2)
		if len(next2) < 2 {
			return nil, false, ErrMalformedBoundary
		}
		endOfPartReached = true
		endOfArchive = string(next2) == "--"
		if !endOfArchive {
			// The boundary was trailed by CRLF; that CRLF now shows up as an
			// empty line. Anything else is malformed.
			if line, ok := s.nextChunk(); !ok || len(line) != 0 {
				return nil, false, ErrMalformedBoundary
			}
		}
	} else {
		// Text encodings: accumulate CRLF lines until a boundary (or EOF for
		// a non-multipart root).
		for chunk, ok := s.nextChunk(); ok; chunk, ok = s.nextChunk() {
			if docBoundary != nil {
				endOfArchive = bytes.Equal(chunk, docBoundary)
			}
			if partBoundary != nil && (bytes.Equal(chunk, partBoundary) || endOfArchive) {
				endOfPartReached = true
				break
			}
			// Each line is decoded UTF-8-with-Latin-1-fallback and re-encoded
			// as UTF-8, so invalid-UTF-8 bytes get Latin-1 → UTF-8 expanded.
			raw = append(raw, decodeUTF8Latin1(chunk)...)
			if encoding == QuotedPrintable {
				// The QP decoder needs CRLF-terminated lines to spot soft
				// breaks.
				raw = append(raw, crlf...)
			}
		}
	}

	if !endOfPartReached && partBoundary != nil {
		// A multipart part must be terminated by a boundary; hitting EOF
		// first is corruption.
		return nil, false, ErrUnterminatedPart
	}

	return &Part{
		ContentType:      header.contentType,
		Charset:          header.charset,
		ContentLocation:  header.contentLocation,
		ContentID:        header.contentID,
		TransferEncoding: encoding,
		raw:              raw,
	}, endOfArchive, nil
}
