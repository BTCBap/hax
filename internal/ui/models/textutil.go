package models

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
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

// wrapWords breaks plain text into lines of at most width cells, breaking
// between words. A word longer than width is kept whole so callers can
// truncate it without losing text silently.
func wrapWords(text string, width int) []string {
	if text == "" {
		return nil
	}
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	var line string
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
