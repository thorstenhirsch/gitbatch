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
	remoteName := "origin"
	if r.State.Remote != nil && r.State.Remote.Name != "" {
		remoteName = r.State.Remote.Name
	}
	switch mode {
	case "fetch":
		msg, err := command.Fetch(r, &command.FetchOptions{
			RemoteName: remoteName,
			Progress:   true,
			Timeout:    command.DefaultFetchTimeout,
		})
		if err != nil {
			command.ScheduleStateEvaluation(r, command.OperationOutcome{
				Operation: command.OperationFetch,
				Err:       err,
			})
			return err
		}
		command.ScheduleStateEvaluation(r, command.OperationOutcome{
			Operation: command.OperationFetch,
			Message:   msg,
		})
		return nil
	case "pull":
		msg, err := command.Pull(r, &command.PullOptions{
			RemoteName:    remoteName,
			Progress:      true,
			ReferenceName: git.UpstreamBranchName(r),
			FFOnly:        true,
		})
		if err != nil {
			command.ScheduleStateEvaluation(r, command.OperationOutcome{
				Operation: command.OperationPull,
				Err:       err,
			})
			return err
		}
		command.ScheduleStateEvaluation(r, command.OperationOutcome{
			Operation: command.OperationPull,
			Message:   msg,
		})
		return nil
	case "merge":
		if r.State.Branch.Upstream == nil {
			return fmt.Errorf("upstream not set")
		}
		msg, err := command.Merge(r, &command.MergeOptions{
			BranchName: r.State.Branch.Upstream.Name,
		})
		if err != nil {
			command.ScheduleStateEvaluation(r, command.OperationOutcome{
				Operation: command.OperationMerge,
				Err:       err,
			})
			return err
		}
		command.ScheduleStateEvaluation(r, command.OperationOutcome{
			Operation: command.OperationMerge,
			Message:   msg,
		})
		return nil
	case "rebase":
		msg, err := command.Pull(r, &command.PullOptions{
			RemoteName:    remoteName,
			Progress:      true,
			ReferenceName: git.UpstreamBranchName(r),
			Rebase:        true,
		})
		if err != nil {
			command.ScheduleStateEvaluation(r, command.OperationOutcome{
				Operation: command.OperationRebase,
				Err:       err,
			})
			return err
		}
		command.ScheduleStateEvaluation(r, command.OperationOutcome{
			Operation: command.OperationRebase,
			Message:   msg,
		})
		return nil
	case "push":
		msg, err := command.Push(r, &command.PushOptions{
			RemoteName:    remoteName,
			ReferenceName: git.UpstreamBranchName(r),
		})
		if err != nil {
			command.ScheduleStateEvaluation(r, command.OperationOutcome{
				Operation: command.OperationPush,
				Err:       err,
			})
			return err
		}
		command.ScheduleStateEvaluation(r, command.OperationOutcome{
			Operation: command.OperationPush,
			Message:   msg,
		})
		return nil
	}
	return fmt.Errorf("unsupported mode: %s", mode)
}

