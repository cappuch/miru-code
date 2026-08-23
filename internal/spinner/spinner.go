package spinner

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/takara-ai/miru-code/internal/terminal"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner is a stderr TTY progress indicator with optional follow lines.
type Spinner struct {
	message    string
	mu         sync.Mutex
	timer      *time.Ticker
	done       chan struct{}
	frame      int
	belowLines int
	stopped    bool
}

// New creates a spinner with the given status message.
func New(message string) *Spinner {
	return &Spinner{message: message}
}

func stderrColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

func paint(code, s string) string {
	if !stderrColorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Start begins the spinner animation (or prints a static line when stderr is not a TTY).
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprintf(os.Stderr, "%s...\n", s.message)
		return
	}
	s.drawLocked()
	s.done = make(chan struct{})
	s.timer = time.NewTicker(80 * time.Millisecond)
	go func() {
		for {
			select {
			case <-s.timer.C:
				s.mu.Lock()
				if !s.stopped {
					s.drawLocked()
				}
				s.mu.Unlock()
			case <-s.done:
				return
			}
		}
	}()
}

// Follow prints a line below the spinner (e.g. Visit URL) while keeping the spinner above.
func (s *Spinner) Follow(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprintln(os.Stderr, line)
		return
	}
	cols := 80
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 {
		cols = w
	}
	rows := 0
	for _, part := range strings.Split(line, "\n") {
		w := terminal.DisplayWidth(part)
		chunk := (w + cols - 1) / cols
		if chunk < 1 {
			chunk = 1
		}
		rows += chunk
	}
	fmt.Fprintf(os.Stderr, "\n%s", line)
	s.belowLines += rows
	fmt.Fprintf(os.Stderr, "\x1b[%dA", s.belowLines)
}

// Stop clears the spinner; optional finalMessage is written in its place.
func (s *Spinner) Stop(finalMessage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
		close(s.done)
		s.timer = nil
	}
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, "\r\x1b[K")
		if finalMessage != "" {
			fmt.Fprint(os.Stderr, finalMessage)
		}
		if s.belowLines > 0 {
			fmt.Fprintf(os.Stderr, "\x1b[%dB", s.belowLines)
		}
		if finalMessage != "" || s.belowLines > 0 {
			fmt.Fprint(os.Stderr, "\n")
		}
	} else if finalMessage != "" {
		fmt.Fprintln(os.Stderr, finalMessage)
	}
}

// Succeed stops with a green checkmark. An empty message clears the spinner silently.
func (s *Spinner) Succeed(message string) {
	if message == "" {
		s.Stop("")
		return
	}
	s.Stop(paint("32", "✓ ") + message)
}

// Fail stops with a red cross message.
func (s *Spinner) Fail(message string) {
	msg := message
	if msg == "" {
		msg = s.message
	}
	s.Stop(paint("31", "✗ ") + msg)
}

func (s *Spinner) drawLocked() {
	glyph := frames[s.frame%len(frames)]
	s.frame++
	fmt.Fprintf(os.Stderr, "\r%s %s", paint("2", glyph), s.message)
}
