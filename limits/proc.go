package limits

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrNotInstalled means the provider's CLI could not be found.
var ErrNotInstalled = errors.New("CLI not found")

var pathOnce sync.Once

// loginPath gives this process the PATH of the user's login shell. An app
// opened from Finder or at login gets launchd's bare PATH, which has
// neither ~/.local/bin (claude) nor Homebrew (codex, node).
func loginPath() {
	pathOnce.Do(func() {
		home, _ := os.UserHomeDir()
		extra := []string{
			filepath.Join(home, ".local/bin"),
			filepath.Join(home, ".claude/local"),
			"/opt/homebrew/bin",
			"/usr/local/bin",
			filepath.Join(home, ".bun/bin"),
			filepath.Join(home, ".npm-global/bin"),
			filepath.Join(home, ".volta/bin"),
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '\n__PATH__%s\n' "$PATH"`).Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if p, ok := strings.CutPrefix(line, "__PATH__"); ok {
					extra = append(strings.Split(p, ":"), extra...)
				}
			}
		}
		seen := map[string]bool{}
		var merged []string
		for _, p := range append(strings.Split(os.Getenv("PATH"), ":"), extra...) {
			if p != "" && !seen[p] {
				seen[p] = true
				merged = append(merged, p)
			}
		}
		os.Setenv("PATH", strings.Join(merged, ":"))
	})
}

func lookPath(name string) (string, error) {
	loginPath()
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s %w", name, ErrNotInstalled)
	}
	return p, nil
}

// session is a CLI talking newline-delimited JSON over stdio.
type session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte
	stderr *strings.Builder
}

func start(ctx context.Context, name string, args ...string) (*session, error) {
	bin, err := lookPath(name)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	// A neutral directory: no project instructions, settings or MCP servers.
	cmd.Dir = os.TempDir()
	cmd.WaitDelay = 2 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &strings.Builder{}
	cmd.Stderr = &limitedWriter{w: stderr, n: 4096}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &session{cmd: cmd, stdin: stdin, lines: make(chan []byte, 16), stderr: stderr}
	go func() {
		defer close(s.lines)
		r := bufio.NewReaderSize(stdout, 1<<20)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				select {
				case s.lines <- line:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return s, nil
}

func (s *session) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.stdin.Write(append(b, '\n'))
	return err
}

// next waits for the first line match accepts.
func (s *session) next(ctx context.Context, match func([]byte) bool) ([]byte, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for the CLI")
		case line, ok := <-s.lines:
			if !ok {
				msg := strings.TrimSpace(s.stderr.String())
				if msg == "" {
					msg = "the CLI exited"
				}
				return nil, errors.New(lastLine(msg))
			}
			if match(line) {
				return line, nil
			}
		}
	}
}

func (s *session) close() {
	s.stdin.Close()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.cmd.Wait()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

type limitedWriter struct {
	mu sync.Mutex
	w  *strings.Builder
	n  int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.n - l.w.Len(); room > 0 {
		if len(p) > room {
			l.w.Write(p[:room])
		} else {
			l.w.Write(p)
		}
	}
	return len(p), nil
}
