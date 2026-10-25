# mhtml-go

A faithful, lossless MHTML (`.mhtml` / `.mht`) parser for Go: a port of the Rust
`mhtml` crate with identical behaviour, quirks
included. No dependencies outside the standard library; Go 1.23+.

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

## Mapping from the Rust crate

| Rust                                 | Go                                         |
| ------------------------------------ | ------------------------------------------ |
| `Archive::parse(&data)`              | `mhtml.Parse(data)`                        |
| `archive.parts()` (`Iterator`)       | `archive.Parts()` (`iter.Seq2[*Part, error]`) |
| `archive.parse_all()` → `OwnedPart`  | `archive.ParseAll()` → `DecodedPart`       |
| `Option<String>` field               | `*string` field                            |
| `Option<SystemTime>` / `Option<&str>` | `(time.Time, bool)` / `(string, bool)`    |
| `Error::MissingBoundary`, …          | `mhtml.ErrMissingBoundary`, …              |

The `Date` header is parsed by a port of the `time` crate's RFC 2822 grammar
rather than `net/mail.ParseDate`, which disagrees on two-digit years, obsolete
zones, comments and leap seconds.

## Development

```bash
go test ./...                  # unit, golden and API tests
go test -fuzz=FuzzParse        # fuzz the parse surface (must never panic)
go vet ./... && gofmt -l .
```

The golden suite (`golden_test.go`) is ported byte-for-byte from the Rust crate
and pins the parser's output; don't weaken it to make a change pass.
