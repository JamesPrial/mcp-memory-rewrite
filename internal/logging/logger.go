package logging

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"time"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	UserIDKey    contextKey = "user_id"
)

// NewLogger creates a new structured logger with the specified service name and level
func NewLogger(service string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize time format
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format(time.RFC3339))
				}
			}
			// Add caller information for errors
			if a.Key == slog.SourceKey {
				if src, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(src.File + ":" + string(rune(src.Line)))
				}
			}
			return a
		},
	}
	
	// Use JSON handler for production, Text handler for development
	// Always log to stderr to avoid interfering with stdio protocol
	var handler slog.Handler
	if os.Getenv("ENV") == "production" || os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	
	return slog.New(handler).With(
		slog.String("service", service),
		slog.Int("pid", os.Getpid()),
		slog.String("go_version", runtime.Version()),
	)
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithUserID adds a user ID to the context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// LoggerWithContext enriches the logger with context values
func LoggerWithContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if ctx == nil {
		return logger
	}
	
	attrs := []slog.Attr{}
	
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	
	if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
		attrs = append(attrs, slog.String("user_id", userID))
	}
	
	if len(attrs) > 0 {
		args := make([]any, 0, len(attrs)*2)
		for _, attr := range attrs {
			args = append(args, attr.Key, attr.Value.Any())
		}
		return logger.With(args...)
	}
	
	return logger
}

// GetLogLevel returns the log level from environment variable
func GetLogLevel() slog.Level {
	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr == "" {
		levelStr = os.Getenv("DEBUG")
		if levelStr == "true" {
			return slog.LevelDebug
		}
	}
	
	switch levelStr {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "info", "INFO":
		return slog.LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}