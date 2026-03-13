package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type TimezoneRunner struct {
	now      func() time.Time
	location *time.Location
	readlink func(string) (string, error)
}

func newTimezoneRunner() *TimezoneRunner {
	return &TimezoneRunner{
		now:      time.Now,
		location: time.Local,
		readlink: os.Readlink,
	}
}

func (r *TimezoneRunner) Run(output io.Writer) error {
	localTime := r.now().In(r.localLocation())
	_, err := fmt.Fprintln(output, r.formatTimezone(localTime))
	return err
}

func (r *TimezoneRunner) localLocation() *time.Location {
	if r.location == nil {
		return time.Local
	}

	return r.location
}

func (r *TimezoneRunner) formatTimezone(now time.Time) string {
	zoneName, offsetSeconds := now.Zone()
	locationName := r.locationName(now.Location(), zoneName)
	offset := formatUTCOffset(offsetSeconds)

	switch {
	case locationName != "" && locationName != "Local" && locationName != zoneName && zoneName != "":
		return fmt.Sprintf("%s (%s, %s)", locationName, zoneName, offset)
	case locationName != "" && locationName != "Local":
		return fmt.Sprintf("%s (%s)", locationName, offset)
	case zoneName != "":
		return fmt.Sprintf("%s (%s)", zoneName, offset)
	default:
		return offset
	}
}

func (r *TimezoneRunner) locationName(location *time.Location, zoneName string) string {
	fallbackName := ""
	if location != nil {
		name := location.String()
		if name != "" && name != "Local" {
			if strings.Contains(name, "/") || name != zoneName {
				return name
			}

			fallbackName = name
		}
	}

	if r.readlink != nil {
		localtimePath, err := r.readlink("/etc/localtime")
		if err == nil {
			if name := parseLocaltimeZonePath(localtimePath); name != "" {
				return name
			}
		}
	}

	if fallbackName != "" {
		return fallbackName
	}

	if location != nil {
		name := location.String()
		if name != "" && name != "Local" {
			return name
		}
	}

	return ""
}

func parseLocaltimeZonePath(localtimePath string) string {
	const zoneInfoMarker = "/zoneinfo/"

	index := strings.Index(localtimePath, zoneInfoMarker)
	if index == -1 {
		return ""
	}

	return strings.TrimPrefix(localtimePath[index+len(zoneInfoMarker):], "/")
}

func formatUTCOffset(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}

	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60

	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}
