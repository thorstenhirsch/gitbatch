package app

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/thorstenhirsch/gitbatch/internal/command"
	"github.com/thorstenhirsch/gitbatch/internal/git"
)

func quick(directories []string, mode string) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0)*4)
	start := time.Now()
	for _, dir := range directories {
		wg.Add(1)
		sem <- struct{}{}
		go func(d string, mode string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := operate(d, mode); err != nil {
				fmt.Fprintf(os.Stderr, "could not perform %s on %s: %s\n", mode, d, err)
				return
			}
			fmt.Printf("%s: successful\n", d)
		}(dir, mode)
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("%d repositories finished in: %s\n", len(directories), elapsed)
	return nil
}

func operate(directory, mode string) error {
	r, err := git.InitializeRepo(directory)
	if err != nil {
		return err
	}
	executor := command.NewExecutor(r)
	switch mode {
	case "fetch":
		return executor.RunFetch(nil, &command.FetchOptions{
			Progress: true,
		})
	case "pull":
		return executor.RunPull(nil, &command.PullOptions{
			Progress: true,
			FFOnly:   true,
		}, false)
	case "merge":
		return executor.RunMerge(nil, nil)
	case "rebase":
		return executor.RunRebase(nil, &command.PullOptions{
			Progress: true,
			Rebase:   true,
		})
	case "push":
		return executor.RunPush(nil, nil, false)
	}
	return fmt.Errorf("unsupported mode: %s", mode)
}
