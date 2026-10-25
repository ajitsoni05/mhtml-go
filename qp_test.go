package mhtml

import "testing"

func TestDecodeQuotedPrintable(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"hex escape", "=3D", "="},
		{"soft line break joins lines", "foo=\r\nbar", "foobar"},
		{"non-hex after equals passes through", "=Z9", "=Z9"},
		{"trailing equals alone is literal", "abc=", "abc="},
		// "=A": only two bytes total from '=', so '=' is literal and 'A' is
		// reconsidered normally (copied verbatim).
		{"trailing equals with one byte is literal", "=A", "=A"},
		{"lowercase hex", "=3d", "="},
		{"empty", "", ""},
		{"golden mixed content", "long line=3Dbar=3D=\r\nmore\r\n", "long line=bar=more\r\n"},
	} {
		if got := string(decodeQuotedPrintable([]byte(tc.in))); got != tc.want {
			t.Errorf("%s: decode(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
