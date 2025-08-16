package ui

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Spinner struct {
	frames   []string
	message  string
	interval time.Duration
	mu       sync.Mutex
	active   bool
	cancel   context.CancelFunc
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames: []string{
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		},
		message:  message,
		interval: 100 * time.Millisecond,
	}
}

func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.active = true

	go s.spin(ctx)
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return
	}

	s.active = false
	if s.cancel != nil {
		s.cancel()
	}

	fmt.Print("\r\033[K")
}

func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

func (s *Spinner) spin(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frameIndex := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.active {
				s.mu.Unlock()
				return
			}

			frame := s.frames[frameIndex]
			message := s.message
			s.mu.Unlock()

			fmt.Printf("\r%s %s %s",
				BrightBlue(frame),
				BrightCyan("thinking..."),
				Dim(message))

			frameIndex = (frameIndex + 1) % len(s.frames)
		}
	}
}

func ShowThinking() *Spinner {
	spinner := NewSpinner("")
	spinner.Start()
	return spinner
}

func ShowToolExecution(toolName string) *Spinner {
	spinner := NewSpinner(fmt.Sprintf("executing %s...", toolName))
	spinner.frames = []string{"|", "/", "-", "\\"}
	spinner.interval = 200 * time.Millisecond
	spinner.Start()
	return spinner
}

func ShowProcessing(message string) *Spinner {
	spinner := NewSpinner(message)
	spinner.frames = []string{"◐", "◓", "◑", "◒"}
	spinner.interval = 150 * time.Millisecond
	spinner.Start()
	return spinner
}
