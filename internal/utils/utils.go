// Package utils provides general-purpose utility functions for the Twitch miner,
// including number formatting and text slugification.
package utils

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var slugifyNonAlnum = regexp.MustCompile(`[^a-z0-9-]+`)

var slugifyMultiHyphen = regexp.MustCompile(`-{2,}`)

// Slugify converts a display name to a URL-friendly slug.
// For example: "Tom Clancy's Rainbow Six Siege" → "tom-clancys-rainbow-six-siege".
func Slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "\u2019", "") // right single quotation mark '
	s = strings.ReplaceAll(s, "\u2018", "") // left single quotation mark '
	s = slugifyNonAlnum.ReplaceAllString(s, "-")
	s = slugifyMultiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// Millify converts a number to a human-readable string with SI suffixes.
// For example: 1000 -> "1K", 1500000 -> "1.5M".
func Millify(n int, precision int) string {
	if precision < 0 {
		precision = 2
	}

	abs := math.Abs(float64(n))
	sign := ""
	if n < 0 {
		sign = "-"
	}

	suffixes := []struct {
		threshold float64
		suffix    string
	}{
		{1e15, "Q"},
		{1e12, "T"},
		{1e9, "B"},
		{1e6, "M"},
		{1e3, "K"},
	}

	for _, s := range suffixes {
		if abs >= s.threshold {
			val := abs / s.threshold
			formatted := formatFloat(val, precision)
			return sign + formatted + s.suffix
		}
	}

	return fmt.Sprintf("%d", n)
}

func formatFloat(f float64, precision int) string {
	s := fmt.Sprintf("%.*f", precision, f)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// Percentage calculates the integer percentage of a/b.
// Returns 0 if a or b is 0.
func Percentage(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return int((float64(a) / float64(b)) * 100)
}

// FloatRound rounds a float to the specified number of decimal places.
func FloatRound(number float64, ndigits int) float64 {
	pow := math.Pow(10, float64(ndigits))
	return math.Round(number*pow) / pow
}
