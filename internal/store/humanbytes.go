package store

import "fmt"

// HumanBytes renders a byte count at its largest 1024-based unit — the single
// owner of that string for every surface (GDK-927). Two implementations used
// to coexist: this one, and a CLI copy that stopped at MB and printed
// "1024.0 MB" for a gigabyte and a negative count verbatim. Callers pass raw
// sizes from the filesystem, so negatives clamp to zero rather than leak a
// "-1 B" into the UI.
func HumanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
