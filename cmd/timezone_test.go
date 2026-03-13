package cmd

import (
	"bytes"
	"testing"
	"time"
)

func TestRootCommandRunsTimezoneSubcommand(t *testing.T) {
	originalLocal := time.Local
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	time.Local = time.FixedZone("Asia/Shanghai", 8*60*60)

	cmd := newRootCmd()
	var output bytes.Buffer

	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"timezone"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if got, want := output.String(), "Asia/Shanghai (UTC+08:00)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
