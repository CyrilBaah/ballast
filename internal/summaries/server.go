package summaries

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"ballast/internal/captions"
)

// Context size and readiness wait (research.md §4, §8).
const (
	ContextTokens = 16384
	readyTimeout  = 3 * time.Minute
)

var (
	// ErrLoadFailed means llama-server couldn't start or load the model.
	ErrLoadFailed = errors.New("summaries: the summary model couldn't be loaded")
	// ErrOutOfMemory means the Mac didn't have the memory to load it.
	ErrOutOfMemory = errors.New("summaries: not enough free memory to write the summary")
)

// Server is a running llama-server bound to this Mac only.
type Server struct {
	URL  string
	cmd  *exec.Cmd
	done chan struct{}

	mu   sync.Mutex
	tail []string // last lines of its stderr, for diagnosis
}

// StartServer runs bin with model on 127.0.0.1 at a free port and waits
// until it answers /health. Cancelling ctx kills it.
func StartServer(ctx context.Context, bin, model string) (*Server, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("summaries: pick a port: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin,
		"-m", model,
		"-c", strconv.Itoa(ContextTokens),
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"-ngl", "99",
		"-np", "1",
	)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLoadFailed, err)
	}
	captions.LowerPriority(cmd.Process.Pid)

	s := &Server{URL: "http://127.0.0.1:" + strconv.Itoa(port), cmd: cmd, done: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			s.mu.Lock()
			s.tail = append(s.tail, sc.Text())
			if len(s.tail) > 30 {
				s.tail = s.tail[len(s.tail)-30:]
			}
			s.mu.Unlock()
		}
		cmd.Wait()
		close(s.done)
	}()

	deadline := time.Now().Add(readyTimeout)
	for {
		select {
		case <-s.done:
			return nil, s.exitError()
		case <-ctx.Done():
			s.Stop()
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
		if resp, err := http.Get(s.URL + "/health"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return s, nil
			}
		}
		if time.Now().After(deadline) {
			s.Stop()
			return nil, fmt.Errorf("%w: it didn't become ready", ErrLoadFailed)
		}
	}
}

// Exited reports whether the server has stopped on its own.
func (s *Server) Exited() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// exitError classifies why the server stopped.
func (s *Server) exitError() error {
	s.mu.Lock()
	tail := strings.ToLower(strings.Join(s.tail, "\n"))
	last := ""
	if len(s.tail) > 0 {
		last = s.tail[len(s.tail)-1]
	}
	s.mu.Unlock()
	if strings.Contains(tail, "out of memory") || strings.Contains(tail, "failed to allocate") {
		return ErrOutOfMemory
	}
	return fmt.Errorf("%w: %s", ErrLoadFailed, last)
}

// Stop kills the server and waits for it to exit.
func (s *Server) Stop() {
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	<-s.done
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
