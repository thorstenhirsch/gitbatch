package command

import (
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
	cmd := exec.Command(c, args...)
	if d != "" {
		cmd.Dir = d
	}
	output, err := cmd.CombinedOutput()
	return trimTrailingNewline(string(output)), err
}

// RunWithTimeout runs a command with a timeout context to prevent hanging
func RunWithTimeout(d string, c string, args []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c, args...)
	if d != "" {
		cmd.Dir = d
	}
	cmd.Env = enrichGitEnv(os.Environ())
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded && runtime.GOOS != "windows" && cmd.Process != nil {
		// Best-effort kill of the entire process group in case children are still running
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return trimTrailingNewline(string(output)), err
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
	cmd := exec.Command(c, args...)
	if d != "" {
		cmd.Dir = d
	}
	var err error
	// this time the execution is a little different
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0

			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				statusCode := status.ExitStatus()
				return statusCode, err
			}
		} else {
			log.Fatalf("cmd.Wait: %v", err)
		}
	}
	return -1, err
}

// trimTrailingNewline removes the trailing new line form a string. this method
// is used mostly on outputs of a command
func trimTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\r") {
		return s[:len(s)-1]
	}
	return s
}
