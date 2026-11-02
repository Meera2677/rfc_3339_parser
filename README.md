# rfc_3339_parser

Parses and validates RFC 3339 timestamps, including fractional seconds and timezone offsets, using only the Go standard library.

```go
package main

import (
	"fmt"
	"log"

	rfc3339parser "rfc_3339_parser"
)

func main() {
	t, err := rfc3339parser.Parse("2024-01-02T03:04:05.123+05:30")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(t.UTC().Format("2006-01-02T15:04:05Z07:00"))
}
```

`Parse` returns a `time.Time` in UTC. It accepts the grammar from RFC 3339 section 5.6 exactly: four-digit years, two-digit fields, an uppercase or lowercase `T` separator, an uppercase or lowercase `Z`, or a signed `hh:mm` offset. Fractional seconds are optional and may have any number of digits; they are truncated to nanosecond precision.

## Why this exists

`time.Parse(time.RFC3339, s)` is close, but it is more permissive than the RFC in some places and less strict in others. It accepts offsets with hours up to 24, it does not reject a missing offset the way the grammar requires, and its error messages do not point at the offending field. This library validates the grammar field by field and returns errors that name the field. The trade-off is that it does not support leap seconds: RFC 3339 allows `:60`, but Go's `time.Time` cannot represent a leap second, so accepting one would silently shift the instant by a second. This library rejects `:60` instead, and the error message says so.

## The awkward edge

The offset is used to compute the instant and then discarded. The returned `time.Time` is always in UTC, so if you need the original offset for display you must parse it separately. This is deliberate: a `time.Time` carries one location, and normalising to UTC makes comparisons and arithmetic unambiguous.

A second edge: fractional seconds longer than nine digits are truncated, not rounded. `...05.1234567899Z` becomes `...05.123456789Z`. Truncation keeps the result at or before the input instant, which is the safer direction for ordering.
