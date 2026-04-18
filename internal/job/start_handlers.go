package job

import (
	"github.com/thorstenhirsch/gitbatch/internal/command"
)

type jobStarter func(*Job) error

var jobStarters = map[Type]jobStarter{
	FetchJob:     startFetchJob,
	PullJob:      startPullJob,
	MergeJob:     startMergeJob,
	RebaseJob:    startRebaseJob,
	PushJob:      startPushJob,
	CommitJob:    startCommitJob,
	StashJob:     startStashJob,
	StashPopJob:  startStashPopJob,
	StashDropJob: startStashDropJob,
}

// The per-operation "running..." status message is set by command.startGitOperation
// when the git queue worker picks up the request; these starters only translate
// the Options payload into the typed command options.

func startFetchJob(j *Job) error {
	return command.NewExecutor(j.Repository).ScheduleFetch(resolveFetchOptions(j.Options))
}

func startPullJob(j *Job) error {
	opts, suppress := resolvePullJobConfig(j.Options)
	return command.NewExecutor(j.Repository).SchedulePull(opts, suppress)
}

func startMergeJob(j *Job) error {
	return command.NewExecutor(j.Repository).ScheduleMerge(nil)
}

func startRebaseJob(j *Job) error {
	opts, _ := resolvePullJobConfig(j.Options)
	return command.NewExecutor(j.Repository).ScheduleRebase(opts)
}

func startPushJob(j *Job) error {
	opts, suppress := resolvePushJobConfig(j.Options)
	return command.NewExecutor(j.Repository).SchedulePush(opts, suppress)
}

func startCommitJob(j *Job) error {
	return command.NewExecutor(j.Repository).ScheduleCommit(resolveCommitOptions(j.Options))
}

func startStashJob(j *Job) error {
	return command.NewExecutor(j.Repository).ScheduleStash(resolveStashOptions(j.Options))
}

func startStashPopJob(j *Job) error {
	return command.NewExecutor(j.Repository).ScheduleStashPop(resolveStashPopOptions(j.Options))
}

func startStashDropJob(j *Job) error {
	return command.NewExecutor(j.Repository).ScheduleStashDrop(resolveStashDropOptions(j.Options))
}

func resolveFetchOptions(options any) *command.FetchOptions {
	switch cfg := options.(type) {
	case nil:
		return nil
	case *command.FetchOptions:
		return cfg
	case command.FetchOptions:
		return &cfg
	default:
		return nil
	}
}

func resolvePullJobConfig(options any) (*command.PullOptions, bool) {
	switch cfg := options.(type) {
	case nil:
		return &command.PullOptions{}, false
	case *command.PullOptions:
		return cfg, false
	case command.PullOptions:
		return &cfg, false
	case PullJobConfig:
		return cfg.Options, cfg.SuppressSuccess
	case *PullJobConfig:
		return cfg.Options, cfg.SuppressSuccess
	default:
		return &command.PullOptions{}, false
	}
}

func resolvePushJobConfig(options any) (*command.PushOptions, bool) {
	switch cfg := options.(type) {
	case nil:
		return &command.PushOptions{}, false
	case *command.PushOptions:
		return cfg, false
	case command.PushOptions:
		return &cfg, false
	case PushJobConfig:
		return cfg.Options, cfg.SuppressSuccess
	case *PushJobConfig:
		return cfg.Options, cfg.SuppressSuccess
	default:
		return &command.PushOptions{}, false
	}
}

func resolveCommitOptions(options any) *command.CommitOptions {
	switch cfg := options.(type) {
	case *command.CommitOptions:
		return cfg
	case command.CommitOptions:
		return &cfg
	default:
		return nil
	}
}

func resolveStashOptions(options any) *command.StashOptions {
	switch cfg := options.(type) {
	case *command.StashOptions:
		return cfg
	case command.StashOptions:
		return &cfg
	default:
		return &command.StashOptions{}
	}
}

func resolveStashPopOptions(options any) *command.StashPopOptions {
	switch cfg := options.(type) {
	case *command.StashPopOptions:
		return cfg
	case command.StashPopOptions:
		return &cfg
	default:
		return nil
	}
}

func resolveStashDropOptions(options any) *command.StashDropOptions {
	switch cfg := options.(type) {
	case *command.StashDropOptions:
		return cfg
	case command.StashDropOptions:
		return &cfg
	default:
		return nil
	}
}
