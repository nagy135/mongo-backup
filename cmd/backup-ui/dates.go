package main

import (
	"sort"
	"strings"
)

// Print one timestamp per archive, including copies with the same capture time.
// Sort independently of filenames so external archives are ordered correctly.
func formatArchiveDates(archives []archive) string {
	if len(archives) == 0 {
		return "No backup archives available.\n"
	}
	ordered := append([]archive(nil), archives...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, _ := ordered[i].backupTime()
		right, _ := ordered[j].backupTime()
		return left.After(right)
	})
	var output strings.Builder
	output.WriteString("Available backup dates (UTC, newest first):\n")
	for _, a := range ordered {
		timestamp, captured := a.backupTime()
		if timestamp.IsZero() {
			output.WriteString("Unknown backup date\n")
			continue
		}
		output.WriteString(timestamp.UTC().Format("2006-01-02 15:04:05 UTC"))
		if !captured {
			output.WriteString(" (file modification time; backup date unknown)")
		}
		output.WriteByte('\n')
	}
	return output.String()
}
