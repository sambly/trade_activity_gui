package logger

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// defaultLogLevel — уровень по умолчанию для сборки, если LOG_LEVEL не задан.
func defaultLogLevel(build string) slog.Level {
	if build == "production" {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// parseLogLevel разбирает значение LOG_LEVEL. ok=false (в т.ч. для пустой строки) —
// переменная не задана или значение не распознано, тогда вызывающий код оставляет
// уровень по умолчанию для сборки.
func parseLogLevel(raw string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return 0, false
	}
}

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

	switch build {
	case "production":
		multiWriter = io.MultiWriter(logFile)
	default:
		multiWriter = io.MultiWriter(os.Stdout, logFile)
	}

	// Уровень по умолчанию зависит от сборки (production — тише), но его можно
	// переопределить переменной окружения LOG_LEVEL (напр. в .env, см. main.go) без
	// пересборки — например, чтобы на уже установленном проде временно включить Debug
	// и поймать проблему с джобом лимитного закрытия (см. position_service.go).
	logLevel := defaultLogLevel(build)
	if override, ok := parseLogLevel(os.Getenv("LOG_LEVEL")); ok {
		logLevel = override
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
