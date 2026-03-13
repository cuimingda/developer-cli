package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

func TestConfigInitCommandCreatesConfigFile(t *testing.T) {
	initializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
	}

	cmd := newRootCmdWithConfigInitializer(initializer)
	var output bytes.Buffer

	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"config", "init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	configPath := initializer.DefaultPath()
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to exist at %q: %v", configPath, err)
	}

	if !strings.Contains(output.String(), configPath) {
		t.Fatalf("expected output to mention %q, got %q", configPath, output.String())
	}
}

func TestConfigInitCommandReturnsErrorWhenConfigFileExists(t *testing.T) {
	initializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
	}

	configPath := initializer.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}

	if err := os.WriteFile(configPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	cmd := newRootCmdWithConfigInitializer(initializer)
	cmd.SetArgs([]string{"config", "init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected Execute() to return an error when config file exists")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists error, got %q", err.Error())
	}
}
