package logger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxLogSize = 9 * 1024 * 1024 // 9 MB

const INFO = "INFO"
const DEBUG = "DEBUG"

type Logger struct {
	file *os.File
	mu   sync.Mutex
}

var (
	loggers       = make(map[string]*Logger)
	mu            sync.Mutex
	level         string = INFO
	globalLogPath string
	globalLogger  *Logger
)

// SetGlobalLogPath sets a single process-wide log file path.
// After this is set, all Get(...) calls return a logger writing to this path.
func SetGlobalLogPath(fileName string) error {
	mu.Lock()
	defer mu.Unlock()

	path, err := normalizePath(fileName)
	if err != nil {
		return err
	}

	globalLogPath = path
	if l, ok := loggers[path]; ok {
		globalLogger = l
	} else {
		globalLogger = nil
	}
	return nil
}

// Get returns a logger bound to a specific file.
// Each file has exactly one logger instance.
func Get(fileName string) *Logger {
	mu.Lock()
	defer mu.Unlock()

	level = INFO

	if globalLogger != nil {
		return globalLogger
	}

	path, err := effectivePath(fileName)
	if err != nil {
		return getFallbackLogger(fileName, err)
	}

	if l, ok := loggers[path]; ok {
		if globalLogPath != "" && path == globalLogPath {
			globalLogger = l
		}
		return l
	}

	if err := ensureParentDir(path); err != nil {
		return getFallbackLogger(path, err)
	}

	if err := rotateIfNeeded(path); err != nil {
		return getFallbackLogger(path, err)
	}

	l, err := newLogger(path)
	if err != nil {
		return getFallbackLogger(path, err)
	}

	loggers[path] = l
	if globalLogPath != "" && path == globalLogPath {
		globalLogger = l
	}
	return l
}

func effectivePath(fileName string) (string, error) {
	if globalLogPath != "" {
		return globalLogPath, nil
	}
	return normalizePath(fileName)
}

func normalizePath(fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", errors.New("log file path cannot be empty")
	}

	path := filepath.Clean(fileName)
	return path, nil
}

func ensureParentDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return nil
}

func getFallbackLogger(cacheKey string, rootErr error) *Logger {
	if l, ok := loggers[cacheKey]; ok {
		return l
	}

	fallbackPath := filepath.Join(os.TempDir(), "guac.log")
	if err := os.MkdirAll(filepath.Dir(fallbackPath), 0755); err == nil {
		if l, err := newLogger(fallbackPath); err == nil {
			fmt.Fprintf(os.Stderr, "log init failed: %v; using fallback log file: %s\n", rootErr, fallbackPath)
			loggers[cacheKey] = l
			if globalLogPath != "" && cacheKey == globalLogPath {
				globalLogger = l
			}
			return l
		}
	}

	fmt.Fprintf(os.Stderr, "log init failed: %v; falling back to stderr output\n", rootErr)
	l := &Logger{file: os.Stderr}
	loggers[cacheKey] = l
	if globalLogPath != "" && cacheKey == globalLogPath {
		globalLogger = l
	}
	return l
}

func rotateIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Size() < maxLogSize {
		return nil
	}

	ts := time.Now().Format("2006_01_02_15_04_05")
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]

	return os.Rename(path, fmt.Sprintf("%s_%s%s", base, ts, ext))
}

func newLogger(path string) (*Logger, error) {
	f, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, err
	}

	return &Logger{file: f}, nil
}

func (l *Logger) log(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("Mon, 02 Jan 2006 15:04:05 MST")
	fmt.Fprintf(l.file, "[%s] [%s] %s\n", ts, level, msg)
}

func (l *Logger) Info(msg string) {
	l.log("INFO", msg)
}

func (l *Logger) Error(msg string) {
	l.log("ERROR", msg)
}

func (l *Logger) Debug(msg string) {
	if level != DEBUG {
		return
	}
	l.log("DEBUG", msg)
}

func (l *Logger) IsDebugEnabled() bool {
	return level == DEBUG
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
