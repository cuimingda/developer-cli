package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	configtemplate "github.com/cuimingda/dev-cli/config"
)

func TestConfigInitializerDefaultPath(t *testing.T) {
	configHome := filepath.Join("/tmp", "Library", "Application Support")
	initializer := &ConfigInitializer{
		configHome:   configHome,
		templateYAML: configtemplate.TemplateYAML(),
	}

	want := filepath.Join(configHome, developerIdentifier, cliName, configFileName)
	if got := initializer.DefaultPath(); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestConfigInitializerInitCreatesTemplateFile(t *testing.T) {
	initializer := &ConfigInitializer{
		configHome:   t.TempDir(),
		templateYAML: configtemplate.TemplateYAML(),
	}

	configPath, err := initializer.Init()
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}

	if got := string(content); got != configtemplate.TemplateYAML() {
		t.Fatalf("config content = %q, want %q", got, configtemplate.TemplateYAML())
	}
}

func TestConfigInitializerInitReturnsErrorWhenConfigExists(t *testing.T) {
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

	_, err := initializer.Init()
	if err == nil {
		t.Fatal("expected Init() to return an error when config file exists")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists error, got %q", err.Error())
	}
}
