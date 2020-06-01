package history

import (
	"bufio"
	"io"
	"strings"
)

// Config contains the History config
type Config struct {
	// OnAppend is called every time a new history line must be persisted
	OnAppend func(line string)

	// Limit indicates how many lines of history to keep
	Limit int
}

// History manages a persistent command line history
type History struct {
	Config
	lines []string
	pos   int
}

// New returns a new History object, reading persisted content
func New(r io.Reader, config *Config) (*History, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > config.Limit {
			lines = lines[len(lines)-config.Limit:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &History{
		Config: *config,
		lines:  lines,
		pos:    len(lines),
	}, nil

}

// Append adds a new line to the command history
func (h *History) Append(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}

	lh := len(h.lines)
	if lh == 0 || h.lines[lh-1] != line {
		h.OnAppend(line)
		h.lines = append(h.lines, line)
		if len(h.lines) > h.Limit {
			h.lines = h.lines[len(h.lines)-h.Limit:]
		}
	}
	h.pos = len(h.lines)
}

// Current returns the currently selected item in the history
func (h *History) Current() string {
	if h.pos >= len(h.lines) || h.pos < 0 {
		return ""
	}
	return h.lines[h.pos]
}

// Up moves the history pointer up and returns the pointed item
func (h *History) Up() string {
	if h.pos > 0 {
		h.pos--
	}
	return h.Current()
}

// Down moves the history pointer up and returns the pointed item
func (h *History) Down() string {
	if h.pos < len(h.lines)-1 {
		h.pos++
		return h.Current()
	}
	h.pos = len(h.lines)
	return ""
}

// Len returns the length of the history, in lines
func (h *History) Len() int {
	return len(h.lines)
}
