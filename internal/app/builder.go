package app

import (
	"fmt"
	"os"

	"github.com/thorstenhirsch/gitbatch/internal/git"
	"github.com/thorstenhirsch/gitbatch/internal/tui"
)

// The App struct is responsible to hold app-wide related entities. Currently
// it has only the gui.Gui pointer for interface entity.
type App struct {
	Config *Config
}

// Config is an assembler data to initiate a setup
type Config struct {
	Directories []string
	LogLevel    string
	Depth       int
	QuickMode   bool
	Mode        string
	Trace       bool
}

// New will handle pre-required operations. It is designed to be a wrapper for
// main method right now.
func New(argConfig *Config) (*App, error) {
	// initiate the app and give it initial values
	app := &App{}
	if len(argConfig.Directories) <= 0 {
		d, _ := os.Getwd()
		argConfig.Directories = []string{d}
	}
	presetConfig, err := loadConfiguration()
	if err != nil {
		return nil, err
	}
	app.Config = overrideConfig(presetConfig, argConfig)

	if err := git.SetTraceLogging(app.Config.Trace); err != nil {
		return nil, err
	}

	return app, nil
}

// Run starts the application.
func (a *App) Run() error {
	dirs := generateDirectories(a.Config.Directories, a.Config.Depth)
	if len(dirs) == 0 {
		return fmt.Errorf("no git repositories found in specified directories")
	}
	if a.Config.QuickMode {
		return a.execQuickMode(dirs)
	}
	// create a tui and run it
	return tui.Run(a.Config.Mode, dirs)
}

func overrideConfig(appConfig, setupConfig *Config) *Config {
	// CLI arguments should always override config file values
	// Only keep appConfig values if setupConfig values are unset/default

	if len(setupConfig.Directories) > 0 {
		appConfig.Directories = setupConfig.Directories
	}
	if len(setupConfig.LogLevel) > 0 {
		appConfig.LogLevel = setupConfig.LogLevel
	}
	// Always use setupConfig.Depth, even if it's 0 (explicit choice)
	// This allows users to override config file with depth=0
	appConfig.Depth = setupConfig.Depth

	if setupConfig.QuickMode {
		appConfig.QuickMode = setupConfig.QuickMode
	}
	if setupConfig.Trace {
		appConfig.Trace = setupConfig.Trace
	}
	if len(setupConfig.Mode) > 0 {
		appConfig.Mode = setupConfig.Mode
	}
	return appConfig
}

func (a *App) execQuickMode(directories []string) error {
	mode := a.Config.Mode
	if mode == "fetch" {
		mode = "pull"
	}
	if mode != "pull" && mode != "merge" && mode != "rebase" {
		return fmt.Errorf("unrecognized quick mode: %s", a.Config.Mode)
	}

	return quick(directories, mode)
}
