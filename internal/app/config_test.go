package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfiguration(t *testing.T) {
	cfg, err := loadConfiguration()
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestReadConfiguration(t *testing.T) {
	err := readConfiguration()
	require.NoError(t, err)
}

func TestInitializeConfigurationManager(t *testing.T) {
	err := initializeConfigurationManager()
	require.NoError(t, err)
}

func TestConfigurationDirectory(t *testing.T) {
	require.NotEmpty(t, configurationDirectory)
	require.Contains(t, configurationDirectory, appName)
}

func TestValidateConfigMode(t *testing.T) {
	validModes := []string{"fetch", "pull", "merge", "rebase", "push"}
	for _, mode := range validModes {
		cfg := &Config{Mode: mode, Depth: 1}
		err := validateConfig(cfg)
		require.NoError(t, err)
		require.Equal(t, mode, cfg.Mode, "valid mode %q should be preserved", mode)
	}

	invalidModes := []string{"", "unknown", "git", "sync"}
	for _, mode := range invalidModes {
		cfg := &Config{Mode: mode, Depth: 1}
		err := validateConfig(cfg)
		require.NoError(t, err)
		require.Equal(t, modeKeyDefault, cfg.Mode, "invalid mode %q should fall back to default", mode)
	}
}
