package models

import "charm.land/bubbles/v2/table"

// naturalTableWidth is the width a table renders at when its cells are left
// unclipped: every visible column plus one cell of padding on each side.
// Bubbles v2 viewports clip content to their width, including width zero, so
// tables that rely on the old unclipped layout must set this explicitly.
func naturalTableWidth(cols []table.Column) int {
	width := 0
	for _, col := range cols {
		if col.Width > 0 {
			width += col.Width + 2
		}
	}
	return width
}
