package cli

import "fmt"

// formatReportDuration renders a completed-duration second count as HH:MM:SS.
func formatReportDuration(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remaining := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, remaining)
}
