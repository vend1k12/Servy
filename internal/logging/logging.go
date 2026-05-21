package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vend1k12/servy/internal/platform"
	"github.com/vend1k12/servy/internal/runner"
)

type Session struct {
	Path string
	file *os.File
}

type Entry struct {
	Timestamp  time.Time     `json:"timestamp"`
	Command    string        `json:"command,omitempty"`
	Profile    string        `json:"profile,omitempty"`
	ConfigPath string        `json:"configPath,omitempty"`
	OS         platform.Info `json:"os"`
	Result     runner.Result `json:"result"`
}

func Open(dir string) (*Session, error) {
	if dir == "" {
		dir = "/var/log/servy"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, time.Now().UTC().Format("20060102T150405Z")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Session{Path: path, file: f}, nil
}

func (s *Session) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	return s.file.Close()
}

func (s *Session) Write(entry Entry) error {
	if s == nil || s.file == nil {
		return nil
	}
	if entry.Result.Step.RedactCommandInLogs {
		entry.Result.Step.Command = []string{"<redacted>"}
		entry.Result.Stdout = "<redacted>"
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write log entry: %w", err)
	}
	return nil
}
