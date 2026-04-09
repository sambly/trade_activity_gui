package logger

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

func SetupLogger(build string) (*os.File, error) {

	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)

	logsDir := filepath.Join(exeDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logsDir, "app.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	var multiWriter io.Writer
	var logLevel slog.Level

	switch build {
	case "production":
		multiWriter = io.MultiWriter(logFile)
		logLevel = slog.LevelInfo
	default:
		multiWriter = io.MultiWriter(os.Stdout, logFile)
		logLevel = slog.LevelDebug
	}

	handler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.SourceKey:
				if source, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(filepath.Base(source.File) + ":" + strconv.Itoa(source.Line))
				}

			case slog.TimeKey:
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05"))
				}
			}
			return a
		},
	})

	slog.SetDefault(slog.New(handler))

	// Стандартный log для совместимости
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return logFile, nil
}

type WailsLoggerAdapter struct {
	l *slog.Logger
}

func NewWailsLoggerAdapter(logger *slog.Logger) *WailsLoggerAdapter {
	wailsLogger := logger.With("component", "wails")
	return &WailsLoggerAdapter{l: wailsLogger}
}

func (w *WailsLoggerAdapter) Print(message string)   { w.l.Info(message) }
func (w *WailsLoggerAdapter) Trace(message string)   { w.l.Debug(message) }
func (w *WailsLoggerAdapter) Debug(message string)   { w.l.Debug(message) }
func (w *WailsLoggerAdapter) Info(message string)    { w.l.Info(message) }
func (w *WailsLoggerAdapter) Warning(message string) { w.l.Warn(message) }
func (w *WailsLoggerAdapter) Error(message string)   { w.l.Error(message) }
func (w *WailsLoggerAdapter) Fatal(message string) {
	w.l.Error(message)
	os.Exit(1)
}

var sensitivePatterns = []*regexp.Regexp{
	// X-Bapi-Api-Key:[abc123]
	regexp.MustCompile(`(?i)(X-Bapi-Api-Key:\[)([^\]]+)(\])`),

	// X-Bapi-Sign:[abcdef...]
	regexp.MustCompile(`(?i)(X-Bapi-Sign:\[)([^\]]+)(\])`),

	// apiKey=abc123 или apiKey: abc123
	regexp.MustCompile(`(?i)(api[-_]?key["']?\s*[:=]\s*["']?)([^"'\s]+)`),

	// secret=xxxx
	regexp.MustCompile(`(?i)(secret["']?\s*[:=]\s*["']?)([^"'\s]+)`),

	// Authorization: Bearer xxx
	regexp.MustCompile(`(?i)(Authorization:\s*Bearer\s+)(\S+)`),
}

func MaskSensitive(input string) string {
	masked := input

	for _, re := range sensitivePatterns {
		masked = re.ReplaceAllString(masked, "${1}***${3}")
	}

	return masked
}
