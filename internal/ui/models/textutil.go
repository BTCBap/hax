package models

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

// prefixRunes returns at most the first n runes of s.
func prefixRunes(s string, n int) string {
	r := []rune(s)
	if n < 0 {
		n = 0
	}
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
