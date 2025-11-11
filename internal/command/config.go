package command

import (
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// ConfigOptions defines the rules for commit operation
type ConfigOptions struct {
	// Section
	Section string
	// Option
	Option string
	// Site should be Global or Local
	Site ConfigSite
}

// ConfigSite defines a string type for the site.
type ConfigSite string

const (
	// ConfigSiteLocal defines a local config.
	ConfigSiteLocal ConfigSite = "local"

	// ConfigSiteGlobal defines a global config.
	ConfigSiteGlobal ConfigSite = "global"
)

// Config adds or reads config of a repository
func Config(r *git.Repository, o *ConfigOptions) (value string, err error) {
	return configWithGit(r, o)
}

// configWithGit is simply a bare git config --site <option>.<section> command which is flexible
func configWithGit(r *git.Repository, options *ConfigOptions) (value string, err error) {
	args := make([]string, 0)
	args = append(args, "config")
	if len(string(options.Site)) > 0 {
		args = append(args, "--"+string(options.Site))
	}
	args = append(args, "--get")
	args = append(args, options.Section+"."+options.Option)
	// parse options to command line arguments
	out, err := Run(r.AbsPath, "git", args)
	if err != nil {
		return out, err
	}
	// till this step everything should be ok
	return out, nil
}
