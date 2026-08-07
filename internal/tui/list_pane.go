package tui

// listPane is a scrollable cursor list shared by the issues, docs, and feed
// panes. cursor indexes navigable items; offset is the first visible
// screen-line index in the rendered line list (headers may shift indices).
type listPane struct {
	cursor int
	offset int
}

// unitRowHeight is the row-height function for flat (one terminal row per
// screen line) lists — issues and feed.
func unitRowHeight(int) int { return 1 }

// move adjusts cursor by delta, clamped to [0, n-1]. When n == 0, cursor is 0.
// Does not scroll; callers that need visibility call ensureVisible next
// (and skip it when n == 0, matching the previous per-pane helpers).
func (p *listPane) move(delta, n int) {
	if n == 0 {
		p.cursor = 0
		return
	}
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= n {
		p.cursor = n - 1
	}
}

// clamp keeps cursor in [0, n-1] (or 0 when n <= 0).
func (p *listPane) clamp(n int) {
	if n <= 0 {
		p.cursor = 0
		return
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= n {
		p.cursor = n - 1
	}
}

// ensureVisible scrolls offset so screen-line line fits in a viewport of
// height terminal rows. rowHeight(i) returns the terminal-row height of
// screen-line i; pass unitRowHeight for flat lists. line is the screen-line
// index of the selected item (may differ from cursor when headers exist).
//
// With a constant row height of 1 this matches the classic:
//
//	if line < offset { offset = line }
//	if line >= offset+height { offset = line - height + 1 }
//
// With variable heights (docs excerpts) it advances offset until the sum of
// heights from offset through line fits in the viewport.
func (p *listPane) ensureVisible(height, line int, rowHeight func(i int) int) {
	if height < 1 {
		height = 1
	}
	if line < p.offset {
		p.offset = line
	}
	if rowHeight == nil {
		rowHeight = unitRowHeight
	}
	for p.offset <= line {
		rows := 0
		for i := p.offset; i <= line; i++ {
			h := rowHeight(i)
			if h < 1 {
				h = 1
			}
			rows += h
		}
		if rows <= height {
			break
		}
		if p.offset >= line {
			break
		}
		p.offset++
	}
	if p.offset < 0 {
		p.offset = 0
	}
}
