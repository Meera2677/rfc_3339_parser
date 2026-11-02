// Package rfc3339parser provides parsing and validation of RFC 3339 timestamps.
//
// The package exposes a single function, Parse, which accepts the exact grammar
// defined by RFC 3339 section 5.6. It rejects timestamps that are syntactically
// valid but semantically impossible (for example, month 13 or day 32), and it
// rejects leap seconds because Go's time package cannot represent them without
// losing information. The trade-off is deliberate: the caller gets a time.Time
// that is safe to use for arithmetic and comparison, at the cost of not being
// able to represent the rare leap-second instant.
package rfc3339parser

import (
	"errors"
	"fmt"
	"time"
)

// Parse parses an RFC 3339 timestamp and returns the corresponding time.Time
// in UTC. It accepts only the exact grammar from RFC 3339 section 5.6:
//
//	date-fullyear   = 4DIGIT
//	date-month      = 2DIGIT  ; 01-12
//	date-mday       = 2DIGIT  ; 01-28, 01-29, 01-30, 01-31 based on month/year
//	time-hour       = 2DIGIT  ; 00-23
//	time-minute     = 2DIGIT  ; 00-59
//	time-second     = 2DIGIT  ; 00-58, 00-59, 00-60 based on leap second rules
//	time-secfrac    = "." 1*DIGIT
//	time-numoffset  = ("+" / "-") time-hour ":" time-minute
//	time-offset     = "Z" / time-numoffset
//	partial-time    = time-hour ":" time-minute ":" time-second [time-secfrac]
//	full-date       = date-fullyear "-" date-month "-" date-mday
//	full-time       = partial-time time-offset
//	date-time       = full-date "T" full-time
//
// The "T" separator may be either uppercase or lowercase, and the "Z" may
// likewise be uppercase or lowercase, because the ABNF permits case-insensitive
// literals. Fractional seconds are accepted with one or more digits; they are
// truncated to nanosecond precision, which is the finest resolution time.Time
// supports. A trailing fractional part without digits (for example, "12:00:00.")
// is rejected.
//
// Leap seconds (second == 60) are rejected. RFC 3339 allows them, but Go's
// time package has no representation for a leap second, so accepting one would
// silently produce a time one second later than the input. Rejecting is the
// honest choice.
//
// The returned time is always in UTC, regardless of the offset in the input.
// The offset is preserved only insofar as it shifts the instant; callers who
// need the original offset should parse it themselves or use a different
// library.
func Parse(s string) (time.Time, error) {
	// The layout is fixed-width, so we can index directly. This avoids the
	// allocations and permissive matching of time.Parse.
	if len(s) < len("2006-01-02T15:04:05Z") {
		return time.Time{}, errors.New("rfc3339parser: timestamp too short")
	}

	year, err := parseUint(s[0:4], 4)
	if err != nil {
		return time.Time{}, fmt.Errorf("rfc3339parser: invalid year: %w", err)
	}
	if s[4] != '-' {
		return time.Time{}, errors.New("rfc3339parser: expected '-' after year")
	}
	month, err := parseUint(s[5:7], 2)
	if err != nil {
		return time.Time{}, fmt.Errorf("rfc3339parser: invalid month: %w", err)
	}
	if s[7] != '-' {
		return time.Time{}, errors.New("rfc3339parser: expected '-' after month")
	}
	day, err := parseUint(s[8:10], 2)
	if err != nil {
		return time.Time{}, fmt.Errorf("rfc3339parser: invalid day: %w", err)
	}

	sep := s[10]
	if sep != 'T' && sep != 't' {
		return time.Time{}, errors.New("rfc3339parser: expected 'T' between date and time")
	}

	hour, err := parseUint(s[11:13], 2)
	if err != nil {
		return time.Time{}, fmt.Errorf("rfc3339parser: invalid hour: %w", err)
	}
	if s[13] != ':' {
		return time.Time{}, errors.New("rfc3339parser: expected ':' after hour")
	}
	minute, err := parseUint(s[14:16], 2)
	if err != nil {
		return time.Time{}, fmt.Errorf("rfc3339parser: invalid minute: %w", err)
	}
	if s[16] != ':' {
		return time.Time{}, errors.New("rfc3339parser: expected ':' after minute")
	}
	second, err := parseUint(s[17:19], 2)
	if err != nil {
		return time.Time{}, fmt.Errorf("rfc3339parser: invalid second: %w", err)
	}

	// Validate the calendar date. time.Date normalises out-of-range values,
	// so we must check them ourselves before constructing the time.
	if month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("rfc3339parser: month %d out of range", month)
	}
	if day < 1 || day > daysInMonth(year, month) {
		return time.Time{}, fmt.Errorf("rfc3339parser: day %d out of range for month %d", day, month)
	}
	if hour > 23 {
		return time.Time{}, fmt.Errorf("rfc3339parser: hour %d out of range", hour)
	}
	if minute > 59 {
		return time.Time{}, fmt.Errorf("rfc3339parser: minute %d out of range", minute)
	}
	if second > 59 {
		// Reject leap seconds explicitly; see the package documentation.
		return time.Time{}, fmt.Errorf("rfc3339parser: second %d out of range (leap seconds are not supported)", second)
	}

	// Fractional seconds are optional. They begin with '.' and run to the
	// end of the string or to the start of the offset.
	nsec := 0
	i := 19
	if i < len(s) && s[i] == '.' {
		i++
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return time.Time{}, errors.New("rfc3339parser: fractional seconds require at least one digit")
		}
		// Truncate to nanosecond precision rather than rounding, so the
		// result is always <= the input instant.
		nsec, err = fracToNsec(s[start:i])
		if err != nil {
			return time.Time{}, fmt.Errorf("rfc3339parser: invalid fractional seconds: %w", err)
		}
	}

	// Offset: either 'Z'/'z' or a signed hh:mm.
	offset := 0
	if i >= len(s) {
		return time.Time{}, errors.New("rfc3339parser: missing timezone offset")
	}
	switch s[i] {
	case 'Z', 'z':
		i++
	case '+', '-':
		if i+6 > len(s) {
			return time.Time{}, errors.New("rfc3339parser: truncated timezone offset")
		}
		sign := 1
		if s[i] == '-' {
			sign = -1
		}
		oh, err := parseUint(s[i+1:i+3], 2)
		if err != nil {
			return time.Time{}, fmt.Errorf("rfc3339parser: invalid offset hour: %w", err)
		}
		if s[i+3] != ':' {
			return time.Time{}, errors.New("rfc3339parser: expected ':' in timezone offset")
		}
		om, err := parseUint(s[i+4:i+6], 2)
		if err != nil {
			return time.Time{}, fmt.Errorf("rfc3339parser: invalid offset minute: %w", err)
		}
		if oh > 23 {
			return time.Time{}, fmt.Errorf("rfc3339parser: offset hour %d out of range", oh)
		}
		if om > 59 {
			return time.Time{}, fmt.Errorf("rfc3339parser: offset minute %d out of range", om)
		}
		offset = sign * (oh*3600 + om*60)
		i += 6
	default:
		return time.Time{}, fmt.Errorf("rfc3339parser: unexpected character %q at offset", s[i])
	}

	if i != len(s) {
		return time.Time{}, fmt.Errorf("rfc3339parser: trailing data %q", s[i:])
	}

	// time.Date with a Location of time.FixedZone gives us the instant in
	// the requested offset; .UTC() then normalises it. We build the location
	// from the offset so that the arithmetic is exact.
	loc := time.UTC
	if offset != 0 {
		loc = time.FixedZone("", offset)
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, nsec, loc)
	return t.UTC(), nil
}

// parseUint parses exactly n ASCII digits from s. It is used instead of
// strconv.Atoi because the RFC grammar requires a fixed width, and Atoi would
// accept leading zeros inconsistently and would not reject a short field.
func parseUint(s string, n int) (int, error) {
	if len(s) != n {
		return 0, fmt.Errorf("expected %d digits, got %d", n, len(s))
	}
	v := 0
	for i := 0; i < n; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit %q", c)
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

// daysInMonth returns the number of days in the given month for the given
// year, accounting for Gregorian leap years.
func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(year) {
			return 29
		}
		return 28
	}
	return 0
}

// isLeapYear reports whether year is a leap year in the proleptic Gregorian
// calendar. The rule is: divisible by 4, except centuries not divisible by 400.
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// fracToNsec converts a string of fractional-second digits to nanoseconds.
// It truncates (does not round) beyond nanosecond precision, so the result is
// always <= the input instant. The input must be all digits and non-empty.
func fracToNsec(digits string) (int, error) {
	// We want the first nine digits, left-padded with zeros if fewer are given.
	// For example, ".5" is 500000000 ns and ".123456789012" truncates to
	// 123456789 ns.
	var buf [9]byte
	for i := range buf {
		buf[i] = '0'
	}
	n := len(digits)
	if n > 9 {
		n = 9
	}
	copy(buf[:n], digits[:n])
	v := 0
	for i := 0; i < 9; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit %q in fractional seconds", c)
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}
