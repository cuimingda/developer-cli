package cmd

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestTimezoneRunnerRunUsesIANANameFromLocaltimeSymlink(t *testing.T) {
	runner := &TimezoneRunner{
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		location: time.FixedZone("CST", 8*60*60),
		readlink: func(path string) (string, error) {
			if path != "/etc/localtime" {
				t.Fatalf("readlink path = %q, want %q", path, "/etc/localtime")
			}

			return "/var/db/timezone/zoneinfo/Asia/Shanghai", nil
		},
	}

	var output bytes.Buffer
	if err := runner.Run(&output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if got, want := output.String(), "Asia/Shanghai (CST, UTC+08:00)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTimezoneRunnerRunFallsBackToZoneAndOffset(t *testing.T) {
	runner := &TimezoneRunner{
		now: func() time.Time {
			return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)
		},
		location: time.FixedZone("NST", -(3*60*60 + 30*60)),
		readlink: func(string) (string, error) {
			return "", errors.New("readlink failed")
		},
	}

	var output bytes.Buffer
	if err := runner.Run(&output); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if got, want := output.String(), "NST (UTC-03:30)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
