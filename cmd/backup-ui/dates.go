package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
	// The Docker runtime does not need a system timezone database.
	_ "time/tzdata"
)

// Print one timestamp per archive, including copies with the same capture time.
// Sort independently of filenames so external archives are ordered correctly.
func formatArchiveDates(archives []archive) (string, error) {
	if len(archives) == 0 {
		return "No backup archives available.\n", nil
	}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return "", fmt.Errorf("cannot load Central European timezone: %w", err)
	}
	ordered := append([]archive(nil), archives...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, _ := ordered[i].backupTime()
		right, _ := ordered[j].backupTime()
		return left.After(right)
	})
	var output strings.Builder
	output.WriteString("Available backup dates (Europe/Berlin, newest first):\n")
	for _, a := range ordered {
		timestamp, captured := a.backupTime()
		if timestamp.IsZero() {
			output.WriteString("Unknown backup date\n")
			continue
		}
		output.WriteString(timestamp.In(location).Format("2006-01-02 15:04:05 MST (-07:00)"))
		if !captured {
			output.WriteString(" (file modification time; backup date unknown)")
		}
		output.WriteByte('\n')
	}
	return output.String(), nil
}
