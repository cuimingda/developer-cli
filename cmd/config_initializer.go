package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	configtemplate "github.com/cuimingda/dev-cli/config"
	"github.com/spf13/viper"
)

const (
	developerIdentifier = "mingda.dev"
	cliName             = "dev"
	configFileName      = "config.yaml"
)

type ConfigInitializer struct {
	configHome   string
	templateYAML string
}

func newDefaultConfigInitializer() *ConfigInitializer {
	return &ConfigInitializer{
		configHome:   xdg.ConfigHome,
		templateYAML: configtemplate.TemplateYAML(),
	}
}

func (c *ConfigInitializer) DefaultPath() string {
	return filepath.Join(c.configHome, developerIdentifier, cliName, configFileName)
}

func (c *ConfigInitializer) Init() (string, error) {
	if strings.TrimSpace(c.configHome) == "" {
		return "", fmt.Errorf("config home is empty")
	}

	configPath := c.DefaultPath()
	if strings.TrimSpace(configPath) == "" {
		return "", fmt.Errorf("default config path is empty")
	}

	if strings.TrimSpace(c.templateYAML) == "" {
		return "", fmt.Errorf("config template is empty")
	}

	if _, err := os.Stat(configPath); err == nil {
		return "", fmt.Errorf("config file already exists: %s", configPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat config file: %w", err)
	}

	validator := viper.New()
	validator.SetConfigType("yaml")
	if err := validator.ReadConfig(strings.NewReader(c.templateYAML)); err != nil {
		return "", fmt.Errorf("parse config template: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(c.templateYAML), 0o644); err != nil {
		return "", fmt.Errorf("write config file: %w", err)
	}

	loadedConfig := viper.New()
	loadedConfig.SetConfigFile(configPath)
	if err := loadedConfig.ReadInConfig(); err != nil {
		return "", fmt.Errorf("read config file: %w", err)
	}

	return configPath, nil
}
