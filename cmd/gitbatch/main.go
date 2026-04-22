package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kingpin"
	"github.com/thorstenhirsch/gitbatch/internal/app"
	"github.com/thorstenhirsch/gitbatch/internal/tui"
)

var version = "dev"

func main() {
	kingpin.Version("gitbatch " + version)
	tui.Version = version

	dirs := kingpin.Flag("directory", "Directory(s) to roam for git repositories.").Short('d').Strings()
	mode := kingpin.Flag("mode", "Operation mode: fetch, pull, merge, rebase, push.").Short('m').String()
	recursionDepth := kingpin.Flag("recursive-depth", "Find directories recursively.").Default("0").Short('r').Int()
	quick := kingpin.Flag("quick", "Runs without gui and fetches/pull remote upstream.").Short('q').Bool()
	trace := kingpin.Flag("trace", "Trace application events to gitbatch.log").Short('t').Bool()

	kingpin.Parse()

	if err := run(*dirs, *recursionDepth, *quick, *mode, *trace); err != nil {
		fmt.Fprintf(os.Stderr, "application quit with an unhandled error: %v", err)
		os.Exit(1)
	}
}

func run(dirs []string, depth int, quick bool, mode string, trace bool) error {
	app, err := app.New(&app.Config{
		Directories: dirs,
		Depth:       depth,
		QuickMode:   quick,
		Mode:        mode,
		Trace:       trace,
	})
	if err != nil {
		return err
	}

	return app.Run()
}
