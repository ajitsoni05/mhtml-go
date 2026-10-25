// Package mhtml is a faithful, lossless MHTML (multipart/related) archive
// parser.
//
// [Parse] reads only the root header; [Archive.Parts] yields parts lazily; each
// [Part.Body] decodes on demand. [Archive.ParseAll] is the strict convenience
// that eagerly decodes every part and fails the whole archive on the first
// error.
package mhtml

import (
	"errors"
	"strings"
	"time"
)

// Errors produced while parsing or decoding an MHTML archive.
//
// Real parsers report all of these as a single whole-archive failure; we
// surface them as distinct sentinels for errors.Is.
var (
	// ErrMissingBoundary: a multipart/* header carried no boundary parameter,
	// so its parts cannot be delimited (the whole archive is rejected here).
	ErrMissingBoundary = errors.New("multipart MIME header is missing its boundary parameter")
	// ErrBinaryPartWithoutBoundary: a binary-encoded part appeared without an
	// enclosing boundary to delimit it (only possible for a non-multipart,
	// IE-style root).
	ErrBinaryPartWithoutBoundary = errors.New("binary part has no boundary to delimit it")
	// ErrUnterminatedPart: end of input was reached before a part's
	// delimiting boundary.
	ErrUnterminatedPart = errors.New("MHTML part is not terminated by a boundary")
	// ErrMalformedBoundary: the bytes trailing a binary part's boundary were
	// malformed (too few bytes to classify, or a non-empty line where the
	// boundary's CRLF was expected).
	ErrMalformedBoundary = errors.New("malformed boundary terminator after binary part")
	// ErrInvalidBase64: a base64-encoded part failed to decode.
	ErrInvalidBase64 = errors.New("invalid base64 content in MHTML part")
)

// TransferEncoding is a part's Content-Transfer-Encoding.
//
// Unknown (and a missing header) are treated as Binary by the parser; a
// yielded [Part] therefore never reports Unknown.
type TransferEncoding int

const (
	QuotedPrintable TransferEncoding = iota
	Base64
	SevenBit
	EightBit
	Binary
	Unknown
)

// Archive is a parsed MHTML archive over the input bytes.
type Archive struct {
	// The parsed root header — source of the creation date, snapshot
	// location, and (for multipart) the boundaries that frame the parts.
	root *mimeHeader
	// Everything after the root header, where the parts begin.
	body []byte
}

// Parse parses the archive's root header. Only the top-level header is read
// here; bodies are decoded on demand. The archive keeps referencing data, so
// callers must not modify it afterwards.
//
// It fails only when the root is a multipart/* type without a boundary
// parameter (the one hard header-level error).
func Parse(data []byte) (*Archive, error) {
	s := newScanner(data, crlf)
	root, err := parseMimeHeader(s)
	if err != nil {
		return nil, err
	}
	return &Archive{root: root, body: s.remaining()}, nil
}

// ParseAll eagerly parses and decodes every part, failing the whole archive on
// the first error (an all-or-nothing contract).
func (a *Archive) ParseAll() ([]DecodedPart, error) {
	var out []DecodedPart
	for part, err := range a.Parts() {
		if err != nil {
			return nil, err
		}
		body, err := part.Body()
		if err != nil {
			return nil, err
		}
		out = append(out, DecodedPart{
			ContentType:      part.ContentType,
			Charset:          part.Charset,
			ContentLocation:  part.ContentLocation,
			ContentID:        part.ContentID,
			TransferEncoding: part.TransferEncoding,
			Body:             body,
		})
	}
	return out, nil
}

// CreationDate returns the archive-level Date header, if present and valid.
// Per-part Date headers are ignored.
//
// Divergence: some parsers expose the creation date only after a *fully
// successful* parse; we expose it straight from the root header. The golden
// date tests all parse successfully, so both agree.
func (a *Archive) CreationDate() (time.Time, bool) {
	if a.root.date == nil {
		return time.Time{}, false
	}
	return *a.root.date, true
}

// SnapshotContentLocation returns the Snapshot-Content-Location header (the
// saved page's URL).
func (a *Archive) SnapshotContentLocation() (string, bool) {
	if a.root.snapshotContentLocation == nil {
		return "", false
	}
	return *a.root.snapshotContentLocation, true
}

// ContentIDToCIDURL translates a Content-ID of the form <foo@bar.com> into a
// cid: URL (cid:foo@bar.com).
//
// It reports false if the content-id is <= 2 bytes long or is not wrapped in
// <...> (rfc2557 §8.3).
func ContentIDToCIDURL(contentID string) (string, bool) {
	if len(contentID) <= 2 {
		return "", false
	}
	if !strings.HasPrefix(contentID, "<") || !strings.HasSuffix(contentID, ">") {
		return "", false
	}
	return "cid:" + contentID[1:len(contentID)-1], true
}
