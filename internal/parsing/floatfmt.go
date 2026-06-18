package parsing

import (
	"math"
	"strconv"
)

// floatErr formats a float32 for inclusion in human-readable error messages.
// Error text is not part of the byte-equal contract (only the serialized
// envelope is), so this only needs to be readable, not identical to C#.
func floatErr(f float32) string {
	return strconv.FormatFloat(float64(f), 'g', -1, 32)
}

// roundedFloatJSON renders a float32 rounded to 4 decimals as it appears in the
// serialized envelope, matching the C# RoundedFloatConverter which writes
// (decimal)Math.Round(value, 4). The decimal cast drops trailing zeros
// (e.g. 2.10 -> "2.1", 13.0 -> "13"), so we format with the minimal digits
// needed for the rounded value.
func roundedFloatJSON(f float32) string {
	// Math.Round in C# uses banker's rounding (round half to even); Go's
	// math.RoundToEven matches it.
	rounded := math.RoundToEven(float64(f)*1e4) / 1e4
	if rounded == 0 {
		// Normalize -0 to 0 so we don't emit "-0" (C# decimal renders 0).
		rounded = 0
	}
	// 'f' with -1 precision then trimming is unsafe for big numbers; use a
	// fixed 4-decimal render and strip trailing zeros / dot, which reproduces
	// the C# decimal formatting for the magnitudes in this domain.
	s := strconv.FormatFloat(rounded, 'f', 4, 64)
	return trimDecimalZeros(s)
}

// trimDecimalZeros removes trailing zeros (and a trailing dot) from a fixed
// decimal string: "2.1000" -> "2.1", "13.0000" -> "13", "0.0000" -> "0".
func trimDecimalZeros(s string) string {
	// Only strings produced by FormatFloat(..,'f',4,..) reach here, so a '.'
	// is always present.
	i := len(s) - 1
	for i >= 0 && s[i] == '0' {
		i--
	}
	if i >= 0 && s[i] == '.' {
		i--
	}
	return s[:i+1]
}
