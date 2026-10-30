package models

import (
	"strings"
	"unicode"
)

// truncateRunes shortens s to at most max runes, replacing the tail with
// ellipsis when it has to cut. It never splits a multi-byte character, so
// labels and error text from users or remote servers render cleanly.
func truncateRunes(s string, max int, ellipsis string) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	keep := max - len([]rune(ellipsis))
	if keep < 0 {
		keep = 0
	}
	return string(r[:keep]) + ellipsis
}

// sanitizeLine drops control characters (newlines, terminal escapes) so
// imported or user-entered text renders on one line and can't restyle
// the terminal.
func sanitizeLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
