package logger

import (
	"io"
	"os"

	"github.com/charmbracelet/log"
)

type Logger struct {
	charm *log.Logger
	file  *os.File
	debug bool // Added this field to fix the "unknown field" error
}

// New sets up the dual-writer logging system
func New(debug bool, logPath string) (*Logger, error) {
	var file *os.File
	var err error

	// 1. Attempt to open the log file if a path is provided
	if logPath != "" {
		file, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		// We don't return nil here; even if the file fails, we want the console logger to work
	}

	// 2. Configure Logger Options
	opts := log.Options{
		ReportTimestamp: true,
		Prefix:          "ShiftLaunch",
	}

	if debug {
		opts.Level = log.DebugLevel
	} else {
		opts.Level = log.InfoLevel
	}

	// 3. Setup MultiWriter
	// This sends logs to the terminal (stderr) AND the file (if it exists)
	var writers []io.Writer
	writers = append(writers, os.Stderr)
	if file != nil {
		writers = append(writers, file)
	}
	multiWriter := io.MultiWriter(writers...)

	// 4. Initialize the Charm logger with our multi-writer
	charmLogger := log.NewWithOptions(multiWriter, opts)

	return &Logger{
		charm: charmLogger,
		file:  file,
		debug: debug,
	}, err // err will be nil unless the file opening failed
}

func (l *Logger) Info(msg string, keyvals ...interface{})  { l.charm.Info(msg, keyvals...) }
func (l *Logger) Debug(msg string, keyvals ...interface{}) { l.charm.Debug(msg, keyvals...) }
func (l *Logger) Error(msg string, keyvals ...interface{}) { l.charm.Error(msg, keyvals...) }
func (l *Logger) Warn(msg string, keyvals ...interface{})  { l.charm.Warn(msg, keyvals...) }

// Capture safely executes a wrapped function.
func (l *Logger) Capture(f func()) {
	f()
}

// TerminalOnly returns an io.Writer that only writes to the console
func (l *Logger) TerminalOnly() io.Writer {
	return os.Stdout
}

// FileOnly returns an io.Writer that only writes to the log file
func (l *Logger) FileOnly() io.Writer {
	if l.file != nil {
		return l.file
	}
	return io.Discard
}