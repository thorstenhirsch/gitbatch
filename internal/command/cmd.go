package command

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	gerr "github.com/thorstenhirsch/gitbatch/internal/errors"
)

var credentialPrompts = []*regexp.Regexp{
	regexp.MustCompile(`Password:`),
	regexp.MustCompile(`.+['’]s password:`),
	regexp.MustCompile(`Password\s*for\s*'.+':`),
	regexp.MustCompile(`Username\s*for\s*'.+':`),
	regexp.MustCompile(`Enter\s*passphrase\s*for\s*key\s*'.+':`),
	regexp.MustCompile(`Enter\s*PIN\s*for\s*.+\s*key\s*.+:`),
	regexp.MustCompile(`Enter\s*PIN\s*for\s*'.+':`),
	regexp.MustCompile(`.*2FA Token.*`),
}

type scanningWriter struct {
	buf      bytes.Buffer
	callback func([]byte)
}

func (w *scanningWriter) Write(p []byte) (n int, err error) {
	if w.callback != nil {
		w.callback(p)
	}
	return w.buf.Write(p)
}

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
	var buf scanningWriter
	credentialDetected := false
	buf.callback = func(p []byte) {
		s := string(p)
		for _, re := range credentialPrompts {
			if re.MatchString(s) {
				credentialDetected = true
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				return
			}
		}
	}
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
		if credentialDetected {
			return trimTrailingNewline(buf.buf.String()), gerr.ErrCredentialPromptDetected
		}
		return trimTrailingNewline(buf.buf.String()), err
	case <-ctx.Done():
		if timer != nil {
			timer.Stop()
		}
		err := <-done
		if credentialDetected {
			return trimTrailingNewline(buf.buf.String()), gerr.ErrCredentialPromptDetected
		}
		if ctx.Err() != nil {
			return trimTrailingNewline(buf.buf.String()), ctx.Err()
		}
		return trimTrailingNewline(buf.buf.String()), err
	case <-timeoutC:
		if timer != nil {
			timer.Stop()
		}
		go func() {
			// If we timed out, it might be because it was waiting for a prompt we missed
			// or just slow.
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}()
		if credentialDetected {
			return trimTrailingNewline(buf.buf.String()), gerr.ErrCredentialPromptDetected
		}
		return trimTrailingNewline(buf.buf.String()), context.DeadlineExceeded
	}
}

func enrichGitEnv(base []string) []string {
	env := make([]string, len(base))
	copy(env, base)
	// We don't disable terminal prompts anymore, because we want to detect them
	// and return a proper error.
	env = ensureEnv(env, "GIT_TERMINAL_PROMPT", "0")
	env = ensureEnv(env, "GIT_SSH_COMMAND", "ssh -o BatchMode=yes -o ConnectTimeout=5 -o ConnectionAttempts=1")
	env = ensureEnv(env, "GIT_HTTP_LOW_SPEED_LIMIT", "1")
	env = ensureEnv(env, "GIT_HTTP_LOW_SPEED_TIME", "10")
	env = ensureEnv(env, "LANG", "C")
	env = ensureEnv(env, "LC_ALL", "C")
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
