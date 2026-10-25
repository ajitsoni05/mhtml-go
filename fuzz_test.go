package mhtml_test

import (
	"testing"

	mhtml "github.com/ajitsoni05/mhtml-go"
)

// FuzzParse feeds arbitrary bytes through the full parser surface, which must
// never panic: the lenient iterator (with per-part Body), the strict
// ParseAll, and the archive-level accessors.
//
//	go test -fuzz=FuzzParse
func FuzzParse(f *testing.F) {
	f.Add(dateArchive("Date: Fri, 1 Mar 2017 22:44:17 -0000\r\n", ""))
	f.Add(corruptArchive)
	f.Fuzz(func(t *testing.T, data []byte) {
		archive, err := mhtml.Parse(data)
		if err != nil {
			return
		}
		archive.CreationDate()
		archive.SnapshotContentLocation()
		for part, err := range archive.Parts() {
			if err == nil {
				_, _ = part.Body()
			}
		}
		_, _ = archive.ParseAll()
	})
}
