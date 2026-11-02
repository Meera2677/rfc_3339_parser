package rfc3339parser

import (
	"testing"
	"time"
)

// TestParseValid covers the shapes that the RFC explicitly allows: uppercase
// and lowercase separators, offsets with both signs, and fractional seconds
// of varying length.
func TestParseValid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{
			name: "utc no fraction",
			in:   "2024-01-02T03:04:05Z",
			want: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		{
			name: "lowercase t and z",
			in:   "2024-01-02t03:04:05z",
			want: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		{
			name: "positive offset",
			in:   "2024-01-02T03:04:05+05:30",
			want: time.Date(2024, 1, 2, 3, 4, 5, 0, time.FixedZone("", 5*3600+30*60)).UTC(),
		},
		{
			name: "negative offset",
			in:   "2024-01-02T03:04:05-08:00",
			want: time.Date(2024, 1, 2, 3, 4, 5, 0, time.FixedZone("", -8*3600)).UTC(),
		},
		{
			name: "fraction one digit",
			in:   "2024-01-02T03:04:05.5Z",
			want: time.Date(2024, 1, 2, 3, 4, 5, 500000000, time.UTC),
		},
		{
			name: "fraction nine digits",
			in:   "2024-01-02T03:04:05.123456789Z",
			want: time.Date(2024, 1, 2, 3, 4, 5, 123456789, time.UTC),
		},
		{
			name: "fraction truncated to nanoseconds",
			in:   "2024-01-02T03:04:05.1234567899Z",
			want: time.Date(2024, 1, 2, 3, 4, 5, 123456789, time.UTC),
		},
		{
			name: "leap day",
			in:   "2024-02-29T00:00:00Z",
			want: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "zero offset explicit",
			in:   "2024-01-02T03:04:05+00:00",
			want: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.in, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("Parse(%q) = %v, want %v", tc.in, got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("Parse(%q) location = %v, want UTC", tc.in, got.Location())
			}
		})
	}
}

// TestParseInvalid covers the malformed inputs that a naive implementation
// often accepts. Each case names the specific rule it violates.
func TestParseInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"too short", "2024-01-02T03:04:05"},
		{"missing offset", "2024-01-02T03:04:05"},
		{"month zero", "2024-00-02T03:04:05Z"},
		{"month thirteen", "2024-13-02T03:04:05Z"},
		{"day zero", "2024-01-00T03:04:05Z"},
		{"day thirty-two", "2024-01-32T03:04:05Z"},
		{"april thirty-one", "2024-04-31T03:04:05Z"},
		{"non-leap february twenty-nine", "2023-02-29T03:04:05Z"},
		{"hour twenty-four", "2024-01-02T24:00:00Z"},
		{"minute sixty", "2024-01-02T03:60:00Z"},
		{"second sixty leap", "2024-01-02T03:04:60Z"},
		{"fraction no digits", "2024-01-02T03:04:05.Z"},
		{"fraction non-digit", "2024-01-02T03:04:05.5aZ"},
		{"offset hour out of range", "2024-01-02T03:04:05+24:00"},
		{"offset minute out of range", "2024-01-02T03:04:05+00:60"},
		{"offset missing colon", "2024-01-02T03:04:05+0500"},
		{"offset truncated", "2024-01-02T03:04:05+05"},
		{"trailing garbage", "2024-01-02T03:04:05Zx"},
		{"space separator", "2024-01-02 03:04:05Z"},
		{"non-digit year", "20x4-01-02T03:04:05Z"},
		{"non-digit month", "2024-0x-02T03:04:05Z"},
		{"non-digit day", "2024-01-0xT03:04:05Z"},
		{"non-digit hour", "2024-01-02T0x:04:05Z"},
		{"non-digit minute", "2024-01-02T03:0x:05Z"},
		{"non-digit second", "2024-01-02T03:04:0xZ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) = %v, want error", tc.in, got)
			}
			if !got.IsZero() {
				t.Errorf("Parse(%q) returned non-zero time %v on error", tc.in, got)
			}
		})
	}
}

// TestParseNormalisesToUTC checks that an offset input yields the same instant
// as the equivalent UTC input, which is the whole point of the offset.
func TestParseNormalisesToUTC(t *testing.T) {
	utc, err := Parse("2024-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("Parse UTC: %v", err)
	}
	offset, err := Parse("2024-01-02T08:34:05+05:30")
	if err != nil {
		t.Fatalf("Parse offset: %v", err)
	}
	if !utc.Equal(offset) {
		t.Errorf("offset instant %v != UTC instant %v", offset, utc)
	}
}

// TestParseLeapYearBoundary checks the Gregorian leap-year rule at the
// century boundary, which is the case a naive year%4 check gets wrong.
func TestParseLeapYearBoundary(t *testing.T) {
	if _, err := Parse("1900-02-29T00:00:00Z"); err == nil {
		t.Error("1900-02-29 should be rejected (1900 is not a leap year)")
	}
	if _, err := Parse("2000-02-29T00:00:00Z"); err != nil {
		t.Errorf("2000-02-29 should be accepted: %v", err)
	}
	if _, err := Parse("2004-02-29T00:00:00Z"); err != nil {
		t.Errorf("2004-02-29 should be accepted: %v", err)
	}
}

// TestParseFractionPadding checks that a short fraction is left-padded rather
// than right-padded. ".5" means half a second, not five nanoseconds.
func TestParseFractionPadding(t *testing.T) {
	got, err := Parse("2024-01-02T03:04:05.5Z")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Nanosecond() != 500000000 {
		t.Errorf("nanosecond = %d, want 500000000", got.Nanosecond())
	}
}
