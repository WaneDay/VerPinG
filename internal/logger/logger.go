package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu   sync.Mutex
	file *os.File
	path string
)

// Init opens (append) the log file app.log inside logDir.
func Init(logDir string) error {
	_ = os.MkdirAll(logDir, 0755)
	path = filepath.Join(logDir, "app.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	file = f
	return nil
}

func write(level, msg string) {
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format("2006-01-02T15:04:05"), level, msg)
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_, _ = file.WriteString(line)
	}
}

// Info logs an informational line.
func Info(msg string) { write("INFO", msg) }

// Warn logs a warning line.
func Warn(msg string) { write("WARN", msg) }

// Error logs an error line.
func Error(msg string) { write("ERROR", msg) }

// Infof logs a formatted informational line.
func Infof(format string, a ...any) { write("INFO", fmt.Sprintf(format, a...)) }

// Path returns the current log file path.
func Path() string { return path }

// ReadTail returns the last max bytes of the log file ("" when unavailable).
func ReadTail(max int) string {
	mu.Lock()
	p := path
	mu.Unlock()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	s := string(b)
	if max > 0 && len(s) > max {
		s = s[len(s)-max:]
	}
	return s
}
