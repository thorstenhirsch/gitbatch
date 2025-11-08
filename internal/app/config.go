package app

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/spf13/viper"
)

// config file stuff
var (
	configFileName = "config"
	configFileExt  = ".yml"
	configType     = "yaml"
	appName        = "gitbatch"

	configurationDirectory = filepath.Join(osConfigDirectory(runtime.GOOS), appName)
	configFileAbsPath      = filepath.Join(configurationDirectory, configFileName)
)

// configuration items
var (
	modeKey             = "mode"
	modeKeyDefault      = "fetch"
	pathsKey            = "paths"
	quickKey            = "quick"
	quickKeyDefault     = false
	recursionKey        = "recursion"
	recursionKeyDefault = 1
)

// Configuration cache to avoid repeated loading
var (
	cachedConfig *Config
	configMutex  sync.RWMutex
	configOnce   sync.Once
)

// loadConfiguration returns a Config struct with caching support
func loadConfiguration() (*Config, error) {
	// Use read lock first to check if we already have cached config
	configMutex.RLock()
	if cachedConfig != nil {
		defer configMutex.RUnlock()
		return cachedConfig, nil
	}
	configMutex.RUnlock()

	// Use sync.Once to ensure configuration is loaded only once
	var err error
	configOnce.Do(func() {
		err = loadConfigurationOnce()
	})

	if err != nil {
		return nil, err
	}

	configMutex.RLock()
	defer configMutex.RUnlock()
	return cachedConfig, nil
}

// loadConfigurationOnce performs the actual configuration loading
func loadConfigurationOnce() error {
	if err := initializeConfigurationManager(); err != nil {
		return err
	}
	if err := setDefaults(); err != nil {
		return err
	}
	if err := readConfiguration(); err != nil {
		return err
	}

	// Build configuration with optimized directory handling
	config, err := buildConfig()
	if err != nil {
		return err
	}

	// Cache the configuration
	configMutex.Lock()
	cachedConfig = config
	configMutex.Unlock()

	return nil
}

// buildConfig creates the Config struct with optimized value retrieval
func buildConfig() (*Config, error) {
	var directories []string
	configPaths := viper.GetStringSlice(pathsKey)

	if len(configPaths) <= 0 {
		// Cache the working directory to avoid repeated system calls
		d, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		directories = []string{d}
	} else {
		directories = configPaths
	}

	config := &Config{
		Directories: directories,
		Depth:       viper.GetInt(recursionKey),
		QuickMode:   viper.GetBool(quickKey),
		Mode:        viper.GetString(modeKey),
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// validateConfig performs basic validation on configuration values
func validateConfig(config *Config) error {
	// Validate depth
	if config.Depth < 0 {
		config.Depth = recursionKeyDefault
	}

	// Validate mode
	if config.Mode != "fetch" && config.Mode != "pull" {
		config.Mode = modeKeyDefault
	}

	// Validate directories exist
	validDirs := make([]string, 0, len(config.Directories))
	for _, dir := range config.Directories {
		if _, err := os.Stat(dir); err == nil {
			validDirs = append(validDirs, dir)
		}
	}
	config.Directories = validDirs

	return nil
}

// set default configuration parameters
func setDefaults() error {
	viper.SetDefault(quickKey, quickKeyDefault)
	viper.SetDefault(recursionKey, recursionKeyDefault)
	viper.SetDefault(modeKey, modeKeyDefault)
	// viper.SetDefault(pathsKey, pathsKeyDefault)
	return nil
}

// read configuration from file with improved error handling
func readConfiguration() error {
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		// Check if file exists more efficiently
		configFile := configFileAbsPath + configFileExt
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			// Create directory and file if they don't exist
			if err = os.MkdirAll(configurationDirectory, 0755); err != nil {
				return err
			}

			// Create the file with minimal operations
			f, err := os.Create(configFile)
			if err != nil {
				return err
			}
			f.Close() // Close immediately, we'll write through viper

			// Write defaults using viper (more efficient than manual file operations)
			if err := viper.WriteConfig(); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

// initialize the configuration manager
func initializeConfigurationManager() error {
	// config viper
	viper.AddConfigPath(configurationDirectory)
	viper.SetConfigName(configFileName)
	viper.SetConfigType(configType)

	return nil
}

// returns OS dependent config directory
func osConfigDirectory(osName string) (osConfigDirectory string) {
	switch osName {
	case "windows":
		osConfigDirectory = os.Getenv("APPDATA")
	case "darwin":
		osConfigDirectory = os.Getenv("HOME") + "/Library/Application Support"
	case "linux":
		osConfigDirectory = os.Getenv("HOME") + "/.config"
	}
	return osConfigDirectory
}
