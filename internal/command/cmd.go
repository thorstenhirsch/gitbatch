package command

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Mode indicates that whether command should run native code or use git
// command to operate.
type Mode uint8

const (
	// ModeLegacy uses traditional git command line tool to operate
	ModeLegacy = iota
	// ModeNative uses native implementation of given git command
	ModeNative
)

// Run runs the OS command and return its output. If the output
// returns error it also encapsulates it as a golang.error which is a return code
// of the command except zero
func Run(d string, c string, args []string) (string, error) {
	return RunWithContext(context.Background(), d, c, args)
}

// RunWithTimeout runs a command with a timeout context to prevent hanging
func RunWithTimeout(d string, c string, args []string, timeout time.Duration) (string, error) {
	return RunWithContextTimeout(context.Background(), d, c, args, timeout)
}

// RunWithContext executes a command honouring the provided context for cancellation.
func RunWithContext(ctx context.Context, d string, c string, args []string) (string, error) {
	return RunWithContextTimeout(ctx, d, c, args, 0)
}

// RunWithContextTimeout executes a command with the supplied context and optional timeout.

func RunWithContextTimeout(ctx context.Context, d string, c string, args []string, timeout time.Duration) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, c, args...)
	if d != "" {
		cmd.Dir = d
	}
	cmd.Env = enrichGitEnv(os.Environ())
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	var timer *time.Timer
	var timeoutC <-chan time.Time
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutC = timer.C
	}
	select {
	case err := <-done:
		if timer != nil {
			timer.Stop()
		}
		return trimTrailingNewline(buf.String()), err
	case <-ctx.Done():
		if timer != nil {
			timer.Stop()
		}
		err := <-done
		if ctx.Err() != nil {
			return trimTrailingNewline(buf.String()), ctx.Err()
		}
		return trimTrailingNewline(buf.String()), err
	case <-timeoutC:
		if timer != nil {
			timer.Stop()
		}
		go func() {
			<-done
		}()
		return trimTrailingNewline(buf.String()), context.DeadlineExceeded
	}
}

func enrichGitEnv(base []string) []string {
	env := make([]string, len(base))
	copy(env, base)
	env = ensureEnv(env, "GIT_TERMINAL_PROMPT", "0")
	env = ensureEnv(env, "GIT_SSH_COMMAND", "ssh -o BatchMode=yes -o ConnectTimeout=5 -o ConnectionAttempts=1")
	env = ensureEnv(env, "GIT_HTTP_LOW_SPEED_LIMIT", "1")
	env = ensureEnv(env, "GIT_HTTP_LOW_SPEED_TIME", "10")
	return env
}

func ensureEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// Return returns if we supposed to get return value as an int of a command
// this method can be used. It is practical when you use a command and process a
// failover according to a specific return code
func Return(d string, c string, args []string) (int, error) {
	return ReturnWithContext(context.Background(), d, c, args)
}

// ReturnWithContext executes a command returning its exit status while honouring context cancellation.
func ReturnWithContext(ctx context.Context, d string, c string, args []string) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, c, args...)
	if d != "" {
		cmd.Dir = d
	}
	cmd.Env = enrichGitEnv(os.Environ())
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				statusCode := status.ExitStatus()
				return statusCode, err
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	}
	return 0, nil
}

// trimTrailingNewline removes the trailing new line form a string. this method
// is used mostly on outputs of a command
func trimTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\r") {
		return s[:len(s)-1]
	}
	return s
}
