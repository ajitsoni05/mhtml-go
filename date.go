package mhtml

import (
	"strings"
	"time"
)

// parseDate parses an RFC 2822 Date header value.
//
// This is a dedicated grammar rather than net/mail.ParseDate, which handles
// two-digit years, named and military zones, comments and leap seconds
// differently. Its quirks are deliberate and pinned by tests (e.g. "-0030" is
// +00:30, "UT" is only recognised at the very end).
//
// Returns nil for anything the grammar rejects. The result is truncated to
// whole seconds in UTC.
func parseDate(value string) *time.Time {
	in := []byte(strings.TrimSpace(value))
	var ok bool

	in = optCFWS(in)
	if len(in) >= 3 && lookupShort(in, weekdays) >= 0 {
		// The weekday is checked for syntax only, never against the date.
		if in, ok = char(in[3:], ','); !ok {
			return nil
		}
		in = optCFWS(in)
	}
	var day int
	if in, day, ok = oneOrTwoDigits(in); !ok {
		return nil
	}
	if in, ok = cfws(in); !ok {
		return nil
	}
	if len(in) < 3 {
		return nil
	}
	month := lookupShort(in, months) + 1
	if month == 0 {
		return nil
	}
	in = in[3:]
	if in, ok = cfws(in); !ok {
		return nil
	}

	var year int
	if rest, y, ok4 := digits(in, 4); ok4 {
		if y < 1900 {
			return nil
		}
		if in, ok = fws(rest); !ok {
			return nil
		}
		year = y
	} else {
		if in, year, ok = digits(in, 2); !ok {
			return nil
		}
		if year < 50 {
			year += 2000
		} else {
			year += 1900
		}
		if in, ok = cfws(in); !ok {
			return nil
		}
	}

	var hour, minute, second int
	if in, hour, ok = digits(in, 2); !ok {
		return nil
	}
	if in, ok = char(optCFWS(in), ':'); !ok {
		return nil
	}
	if in, minute, ok = digits(optCFWS(in), 2); !ok {
		return nil
	}
	if rest, hasColon := char(optCFWS(in), ':'); hasColon {
		if in, second, ok = digits(optCFWS(rest), 2); !ok {
			return nil
		}
	}
	if in, ok = cfws(in); !ok {
		return nil
	}

	var offHour, offMinute int
	if len(in) > 0 && (in[0] == '+' || in[0] == '-') {
		negative := in[0] == '-'
		if in, offHour, ok = digits(in[1:], 2); !ok {
			return nil
		}
		if in, offMinute, ok = digits(in, 2); !ok {
			return nil
		}
		if negative {
			offHour = -offHour
		}
	} else if in, offHour, ok = zoneLiteral(in); !ok {
		return nil
	}
	if len(optCFWS(in)) != 0 {
		return nil
	}

	// A leap second is accepted only as a stand-in for the last second of a
	// month in UTC, read as hh:mm:59.999999999.
	leap := second == 60
	nanos := 0
	if leap {
		second, nanos = 59, 999_999_999
	}
	if day < 1 || day > daysIn(time.Month(month), year) ||
		hour > 23 || minute > 59 || second > 59 ||
		offHour < -25 || offHour > 25 || offMinute > 59 {
		return nil
	}
	// The minute's sign follows the hour's; with a zero hour it stays
	// positive, which is why "-0030" means +00:30.
	if offHour < 0 {
		offMinute = -offMinute
	}
	zone := time.FixedZone("", offHour*3600+offMinute*60)
	t := time.Date(year, time.Month(month), day, hour, minute, second, nanos, zone)
	if leap {
		u := t.UTC()
		if u.Hour() != 23 || u.Minute() != 59 || u.Second() != 59 ||
			u.Day() != daysIn(u.Month(), u.Year()) {
			return nil
		}
	}
	t = time.Unix(t.Unix(), 0).UTC()
	return &t
}

func daysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

var (
	weekdays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	months   = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// lookupShort matches the first three bytes of in ASCII case-insensitively
// against names, returning the index or -1. len(in) must be at least 3.
func lookupShort(in []byte, names []string) int {
	prefix := asciiLower(string(in[:3]))
	for i, name := range names {
		if prefix == asciiLower(name) {
			return i
		}
	}
	return -1
}

func char(in []byte, c byte) ([]byte, bool) {
	if len(in) > 0 && in[0] == c {
		return in[1:], true
	}
	return in, false
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

// digits consumes exactly n ASCII digits.
func digits(in []byte, n int) ([]byte, int, bool) {
	if len(in) < n {
		return in, 0, false
	}
	v := 0
	for _, c := range in[:n] {
		if !isDigit(c) {
			return in, 0, false
		}
		v = v*10 + int(c-'0')
	}
	return in[n:], v, true
}

func oneOrTwoDigits(in []byte) ([]byte, int, bool) {
	if rest, v, ok := digits(in, 2); ok {
		return rest, v, true
	}
	return digits(in, 1)
}

// zoneLiteral consumes an obsolete zone name, returning its hour offset.
// "UT" and the single-letter military zones (all read as +00, except the
// unassigned J) are only recognised when fewer than three bytes remain.
func zoneLiteral(in []byte) ([]byte, int, bool) {
	if len(in) < 3 {
		if len(in) == 0 {
			return in, 0, false
		}
		if len(in) == 2 && (in[0] == 'u' || in[0] == 'U') && (in[1] == 't' || in[1] == 'T') {
			return in[2:], 0, true
		}
		c := in[0]
		if ('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z') && c != 'j' && c != 'J' {
			return in[1:], 0, true
		}
		return in, 0, false
	}
	switch asciiLower(string(in[:3])) {
	case "gmt":
		return in[3:], 0, true
	case "edt":
		return in[3:], -4, true
	case "est", "cdt":
		return in[3:], -5, true
	case "cst", "mdt":
		return in[3:], -6, true
	case "mst", "pdt":
		return in[3:], -7, true
	case "pst":
		return in[3:], -8, true
	}
	return in, 0, false
}

func isWSP(c byte) bool { return c == ' ' || c == '\t' }

func oneOrMoreWSP(in []byte) ([]byte, bool) {
	if len(in) == 0 || !isWSP(in[0]) {
		return in, false
	}
	for len(in) > 0 && isWSP(in[0]) {
		in = in[1:]
	}
	return in, true
}

// fws consumes folding white space. The single-byte fast path mirrors the
// reference exactly, including consuming a lone CR.
func fws(in []byte) ([]byte, bool) {
	if len(in) == 0 || (in[0] != '\r' && !isWSP(in[0])) {
		return in, false
	}
	if len(in) < 2 || (in[1] != '\r' && in[1] != '\n' && !isWSP(in[1])) {
		return in[1:], true
	}
	if in[0] == '\r' && in[1] == '\n' {
		return oneOrMoreWSP(in[2:])
	}
	orig := in
	in, ok := oneOrMoreWSP(in)
	if !ok {
		return orig, false
	}
	for len(in) >= 2 && in[0] == '\r' && in[1] == '\n' {
		if in, ok = oneOrMoreWSP(in[2:]); !ok {
			return orig, false
		}
	}
	return in, true
}

// cfws consumes a non-empty run of folding white space and comments.
func cfws(in []byte) ([]byte, bool) {
	if len(in) == 0 {
		return in, false
	}
	if in[0] != '(' {
		if in[0] != '\r' && !isWSP(in[0]) {
			return in, false
		}
		if len(in) < 2 || (in[1] != '(' && in[1] != '\r' && in[1] != '\n' && !isWSP(in[1])) {
			return in[1:], true
		}
	}
	matched := false
	for {
		rest, ok := fws(in)
		if !ok {
			rest, ok = comment(in, 1)
		}
		if !ok {
			return in, matched
		}
		in, matched = rest, true
	}
}

func optCFWS(in []byte) []byte {
	in, _ = cfws(in)
	return in
}

func zeroOrMoreFWS(in []byte) []byte {
	for {
		rest, ok := fws(in)
		if !ok {
			return in
		}
		in = rest
	}
}

// commentDepthLimit bounds comment nesting so hostile input can't exhaust the
// stack.
const commentDepthLimit = 32

func comment(in []byte, depth int) ([]byte, bool) {
	if depth == commentDepthLimit {
		return in, false
	}
	orig := in
	in, ok := char(in, '(')
	if !ok {
		return orig, false
	}
	in = zeroOrMoreFWS(in)
	for {
		rest, ok := ccontent(in, depth+1)
		if !ok {
			break
		}
		in = zeroOrMoreFWS(rest)
	}
	if in, ok = char(in, ')'); !ok {
		return orig, false
	}
	return in, true
}

func ccontent(in []byte, depth int) ([]byte, bool) {
	if rest, ok := ctext(in); ok {
		return rest, true
	}
	if rest, ok := quotedPair(in); ok {
		return rest, true
	}
	return comment(in, depth)
}

func ctext(in []byte) ([]byte, bool) {
	if len(in) == 0 {
		return in, false
	}
	switch c := in[0]; {
	case 1 <= c && c <= 8, c == 11, c == 12, 14 <= c && c <= 31, c == 127, // obs-NO-WS-CTL
		33 <= c && c <= 39, 42 <= c && c <= 91, 93 <= c && c <= 126:
		return in[1:], true
	}
	return in, false
}

func quotedPair(in []byte) ([]byte, bool) {
	in, ok := char(in, '\\')
	if !ok {
		return in, false
	}
	return text(in), true
}

// text consumes one text byte, or else an obs-text run. It always succeeds,
// possibly consuming nothing.
func text(in []byte) []byte {
	if len(in) > 0 {
		if c := in[0]; c != 0 && c != '\n' && c != '\r' && c <= 127 {
			return in[1:]
		}
	}
	skipLFCR := func(in []byte) []byte {
		for len(in) > 0 && in[0] == '\n' {
			in = in[1:]
		}
		for len(in) > 0 && in[0] == '\r' {
			in = in[1:]
		}
		return in
	}
	in = skipLFCR(in)
	// obs-char: any ASCII but LF/CR, stopping at ')' so the comment can close.
	for len(in) > 0 && in[0] != ')' && in[0] != '\n' && in[0] != '\r' && in[0] <= 127 {
		in = skipLFCR(in[1:])
	}
	return in
}
