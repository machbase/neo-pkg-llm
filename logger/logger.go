package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level represents log severity.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var levelNames = [...]string{"DEBUG", "INFO", "WARN", "ERROR"}

func (l Level) String() string {
	if l >= DEBUG && l <= ERROR {
		return levelNames[l]
	}
	return "UNKNOWN"
}

// ParseLevel converts a string to Level. Defaults to INFO.
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// Options configures the logger.
type Options struct {
	Dir        string // log file directory (default: "logs")
	FilePrefix string // log file prefix (default: "app")
	Level      Level  // minimum log level (default: INFO)
	MaxSizeMB  int    // max file size in MB before rotation (default: 10)
	MaxFiles   int    // max rotated files to keep (default: 5)
	ToStdout   bool   // also write to stdout (default: true)
}

func defaultOptions() Options {
	return Options{
		Dir:        "logs",
		FilePrefix: "app",
		Level:      INFO,
		MaxSizeMB:  10,
		MaxFiles:   5,
		ToStdout:   true,
	}
}

// Logger is the main logger with file rotation support.
type Logger struct {
	mu       sync.Mutex
	opts     Options
	file     *os.File
	fileSize int64
	maxBytes int64
}

// New creates a new Logger. Passing nil uses defaults.
func New(opts *Options) (*Logger, error) {
	o := defaultOptions()
	if opts != nil {
		if opts.Dir != "" {
			o.Dir = opts.Dir
		}
		if opts.FilePrefix != "" {
			o.FilePrefix = opts.FilePrefix
		}
		if opts.MaxSizeMB > 0 {
			o.MaxSizeMB = opts.MaxSizeMB
		}
		if opts.MaxFiles > 0 {
			o.MaxFiles = opts.MaxFiles
		}
		o.Level = opts.Level
		o.ToStdout = opts.ToStdout
	}

	if err := os.MkdirAll(o.Dir, 0755); err != nil {
		return nil, fmt.Errorf("logger: failed to create dir %s: %w", o.Dir, err)
	}

	l := &Logger{
		opts:     o,
		maxBytes: int64(o.MaxSizeMB) * 1024 * 1024,
	}

	if err := l.openFile(); err != nil {
		return nil, err
	}

	return l, nil
}

// currentFileName returns the active log file name.
func (l *Logger) currentFileName() string {
	return filepath.Join(l.opts.Dir, l.opts.FilePrefix+".log")
}

// openFile opens (or creates) the current log file.
func (l *Logger) openFile() error {
	name := l.currentFileName()
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("logger: failed to open %s: %w", name, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("logger: failed to stat %s: %w", name, err)
	}
	l.file = f
	l.fileSize = info.Size()
	return nil
}

// rotate renames current file with timestamp and opens a new one.
func (l *Logger) rotate() error {
	if l.file != nil {
		l.file.Close()
	}

	current := l.currentFileName()
	ts := time.Now().Format("20060102_150405")
	rotated := filepath.Join(l.opts.Dir, fmt.Sprintf("%s_%s.log", l.opts.FilePrefix, ts))
	os.Rename(current, rotated)

	l.cleanup()
	return l.openFile()
}

// cleanup removes old rotated files exceeding MaxFiles.
func (l *Logger) cleanup() {
	pattern := filepath.Join(l.opts.Dir, l.opts.FilePrefix+"_*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= l.opts.MaxFiles {
		return
	}
	sort.Strings(matches) // oldest first (timestamp in name)
	for _, f := range matches[:len(matches)-l.opts.MaxFiles] {
		os.Remove(f)
	}
}

// log is the core write method.
func (l *Logger) log(level Level, component, format string, args ...interface{}) {
	if level < l.opts.Level {
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)

	var line string
	if component != "" {
		line = fmt.Sprintf("%s [%-5s] [%s] %s\n", now, level, component, msg)
	} else {
		line = fmt.Sprintf("%s [%-5s] %s\n", now, level, msg)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// write to stdout
	if l.opts.ToStdout {
		io.WriteString(os.Stdout, line)
	}

	// write to file
	if l.file != nil {
		n, _ := io.WriteString(l.file, line)
		l.fileSize += int64(n)

		if l.maxBytes > 0 && l.fileSize >= l.maxBytes {
			l.rotate()
		}
	}
}

// SetLevel changes the minimum log level at runtime.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	l.opts.Level = level
	l.mu.Unlock()
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// --- Logging methods ---

func (l *Logger) Debug(format string, args ...interface{})  { l.log(DEBUG, "", format, args...) }
func (l *Logger) Info(format string, args ...interface{})   { l.log(INFO, "", format, args...) }
func (l *Logger) Warn(format string, args ...interface{})   { l.log(WARN, "", format, args...) }
func (l *Logger) Error(format string, args ...interface{})  { l.log(ERROR, "", format, args...) }

// Component returns a sub-logger with a fixed component tag.
func (l *Logger) Component(name string) *ComponentLogger {
	return &ComponentLogger{parent: l, component: name}
}

// ComponentLogger is a logger bound to a specific component tag.
type ComponentLogger struct {
	parent    *Logger
	component string
}

func (c *ComponentLogger) Debug(format string, args ...interface{}) {
	c.parent.log(DEBUG, c.component, format, args...)
}
func (c *ComponentLogger) Info(format string, args ...interface{}) {
	c.parent.log(INFO, c.component, format, args...)
}
func (c *ComponentLogger) Warn(format string, args ...interface{}) {
	c.parent.log(WARN, c.component, format, args...)
}
func (c *ComponentLogger) Error(format string, args ...interface{}) {
	c.parent.log(ERROR, c.component, format, args...)
}

// Fatal logs at ERROR level and exits.
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(ERROR, "", format, args...)
	l.Close()
	os.Exit(1)
}

// --- Default global logger ---

var std *Logger

func init() {
	// fallback: stdout-only logger if Init() is never called
	std = &Logger{
		opts: Options{
			Level:    INFO,
			ToStdout: true,
		},
	}
}

// Init initializes the global logger. Call once at startup.
func Init(opts *Options) error {
	l, err := New(opts)
	if err != nil {
		return err
	}
	std = l
	return nil
}

// Std returns the global logger instance.
func Std() *Logger { return std }

// Package-level convenience functions using the global logger.

func Debug(format string, args ...interface{})  { std.Debug(format, args...) }
func Infof(format string, args ...interface{})  { std.Info(format, args...) }
func Warnf(format string, args ...interface{})  { std.Warn(format, args...) }
func Errorf(format string, args ...interface{}) { std.Error(format, args...) }
func Fatalf(format string, args ...interface{}) { std.Fatal(format, args...) }

// Caller returns "file:line" of the caller (skip frames up).
func Caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "???:0"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
