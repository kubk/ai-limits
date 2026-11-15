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

// Prepare gives this process the PATH of the user's login shell, once. An
// app opened from Finder or at login gets launchd's bare PATH, which has
// neither ~/.local/bin (claude) nor Homebrew (codex, node). It starts a
// login shell, so call it early, off the main thread.
func Prepare() {
	pathOnce.Do(func() {
		path := loginShellPath()
		if path == "" {
			// No usable shell: the usual install locations.
			home, _ := os.UserHomeDir()
			path = strings.Join([]string{
				os.Getenv("PATH"),
				filepath.Join(home, ".local/bin"),
				"/opt/homebrew/bin",
				"/usr/local/bin",
			}, ":")
		}
		os.Setenv("PATH", path)
	})
}

func loginShellPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Interactive too, for version managers that set PATH in .zshrc.
	out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '\n__PATH__%s\n' "$PATH"`).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "__PATH__"); ok {
			return p
		}
	}
	return ""
}

// message is one JSON line of a CLI, decoded as far as replies are matched.
type message struct {
	raw      []byte
	ID       *int   `json:"id"`     // JSON-RPC (codex)
	Method   string `json:"method"` // set on the server's own requests
	Type     string `json:"type"`   // stream-json (claude)
	Response struct {
		RequestID string `json:"request_id"`
	} `json:"response"`
}

// session is a CLI talking newline-delimited JSON over stdio.
type session struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	msgs   chan message
	stderr *limitedWriter
}

func start(ctx context.Context, name string, args ...string) (*session, error) {
	Prepare()
	bin, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s %w", name, ErrNotInstalled)
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
	s := &session{cmd: cmd, stdin: stdin, msgs: make(chan message, 16), stderr: &limitedWriter{n: 4096}}
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		defer close(s.msgs)
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadBytes('\n')
			var m message
			if json.Unmarshal(line, &m) == nil {
				m.raw = line
				select {
				case s.msgs <- m:
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

// call sends req and waits for the reply match accepts, which it decodes
// into out.
func (s *session) call(ctx context.Context, req any, match func(message) bool, out any) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := s.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	if match == nil {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for the CLI")
		case m, ok := <-s.msgs:
			if !ok {
				return errors.New(s.stderr.lastLine())
			}
			if match(m) {
				return json.Unmarshal(m.raw, out)
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

// limitedWriter keeps the first n bytes of what a CLI writes to stderr.
type limitedWriter struct {
	mu sync.Mutex
	b  strings.Builder
	n  int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.n - l.b.Len(); room > 0 {
		l.b.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (l *limitedWriter) lastLine() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := strings.TrimSpace(l.b.String())
	if s == "" {
		return "the CLI exited"
	}
	return strings.TrimSpace(s[strings.LastIndex(s, "\n")+1:])
}
