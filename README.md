# mhtml-go

A faithful, lossless MHTML (`.mhtml` / `.mht`) parser for Go. No dependencies
outside the standard library; Go 1.23+.

```go
archive, err := mhtml.Parse(data) // reads only the root header
if err != nil {
	return err // only a multipart root without a boundary fails here
}
for part, err := range archive.Parts() { // lazy; one error, then stops
	if err != nil {
		return err
	}
	body, err := part.Body() // decoded on demand (base64 / quoted-printable)
	// part.ContentType, part.Charset, part.ContentLocation, part.ContentID ...
}

parts, err := archive.ParseAll() // strict: all parts decoded, or an error
date, ok := archive.CreationDate()
url, ok := archive.SnapshotContentLocation()
cid, ok := mhtml.ContentIDToCIDURL("<foo@bar.com>") // "cid:foo@bar.com"
```

Errors are sentinels (`ErrMissingBoundary`, `ErrUnterminatedPart`, …) for
`errors.Is`. Optional header fields are `*string` (nil when absent, so an empty
header stays distinguishable). Binary bodies alias the input without copying:
don't modify `data` after `Parse`, or the slices `Body` returns.

## Development

```bash
go test ./...                  # unit, golden and API tests
go test -fuzz=FuzzParse        # fuzz the parse surface (must never panic)
go vet ./... && gofmt -l .
```

The golden suite (`golden_test.go`) pins the parser's byte-exact output; don't
weaken it to make a change pass.
