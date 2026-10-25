package mhtml

// decodeQuotedPrintable reproduces real-world browser leniency
// (bug-compatibility over RFC 2045 strictness): an '=' with fewer than two
// trailing bytes, or followed by non-hex, is emitted verbatim rather than
// rejected. Infallible.
func decodeQuotedPrintable(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		c := data[i]
		// A '=xx' sequence requires at least three bytes *total* from the '=';
		// otherwise the '=' is literal and the trailing 1-2 bytes are
		// reconsidered on the next iterations (they are NOT consumed here).
		if c != '=' || len(data)-i < 3 {
			out = append(out, c)
			i++
			continue
		}
		upper, lower := data[i+1], data[i+2]
		i += 3
		if upper == '\r' && lower == '\n' {
			// Soft line break — emit nothing.
			continue
		}
		u, uok := hexValue(upper)
		l, lok := hexValue(lower)
		if uok && lok {
			out = append(out, u<<4|l)
		} else {
			// Invalid: '=' followed by non-hex — emit all three bytes verbatim.
			out = append(out, '=', upper, lower)
		}
	}
	return out
}

func hexValue(b byte) (byte, bool) {
	switch {
	case '0' <= b && b <= '9':
		return b - '0', true
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10, true
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}
