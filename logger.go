package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// Logger is the interface used for logging by flagsmith client. This interface defines the methods
// that a logger implementation must implement. It is used to abstract logging and
// enable clients to use any logger implementation they want.
type Logger interface {
	// Errorf logs an error message with the given format and arguments.
	Errorf(format string, v ...interface{})

	// Warnf logs a warning message with the given format and arguments.
	Warnf(format string, v ...interface{})

	// Debugf logs a debug message with the given format and arguments.
	Debugf(format string, v ...interface{})
}

// slogToRestyAdapter adapts a slog.Logger to resty.Logger.
type slogToRestyAdapter struct {
	logger *slog.Logger
}

func newSlogToRestyAdapter(logger *slog.Logger) *slogToRestyAdapter {
	return &slogToRestyAdapter{logger: logger}
}

func (l *slogToRestyAdapter) Errorf(format string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

func (l *slogToRestyAdapter) Warnf(format string, v ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, v...))
}

func (l *slogToRestyAdapter) Debugf(format string, v ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}

// slogToLoggerAdapter adapts a slog.Logger to our Logger interface.
type slogToLoggerAdapter struct {
	logger *slog.Logger
}

func newSlogToLoggerAdapter(logger *slog.Logger) *slogToLoggerAdapter {
	return &slogToLoggerAdapter{logger: logger}
}

func (l *slogToLoggerAdapter) Errorf(format string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

func (l *slogToLoggerAdapter) Warnf(format string, v ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, v...))
}

func (l *slogToLoggerAdapter) Debugf(format string, v ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}

// loggerToSlogAdapter adapts our Logger interface to a slog.Logger.
type loggerToSlogAdapter struct {
	logger Logger
}

func newLoggerToSlogAdapter(logger Logger) *slog.Logger {
	return slog.New(&loggerToSlogAdapter{logger: logger})
}

// implement slog.Handler interface to adapt our Logger interface to a slog.Logger

func (l *loggerToSlogAdapter) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (l *loggerToSlogAdapter) Handle(ctx context.Context, r slog.Record) error {
	msg := r.Message
	var attrs strings.Builder
	r.Attrs(func(a slog.Attr) bool {
		attrs.WriteString(a.String() + " ")
		return true
	})
	msg += attrs.String()

	switch r.Level {
	case slog.LevelError:
		l.logger.Errorf(msg)
	case slog.LevelWarn:
		l.logger.Warnf(msg)
	case slog.LevelDebug:
		l.logger.Debugf(msg)
	}
	return nil
}

func (l *loggerToSlogAdapter) WithAttrs(_ []slog.Attr) slog.Handler {
	return l
}

func (l *loggerToSlogAdapter) WithGroup(_ string) slog.Handler {
	return l
}

func createLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

const (
	contextLoggerKey    contextKey = contextKey("logger")
	contextStartTimeKey contextKey = contextKey("startTime")
)

func newRestyLogRequestMiddleware(logger *slog.Logger) resty.RequestMiddleware {
	return func(c *resty.Client, req *resty.Request) error {
		// Create a child logger with request metadata
		reqLogger := logger.WithGroup("http").With(
			"method", req.Method,
			"url", req.URL,
		)
		reqLogger.Debug("request")

		// Store the logger in this request's context, and use it in the response
		req.SetContext(context.WithValue(req.Context(), contextLoggerKey, reqLogger))

		// Time the current request
		req.SetContext(context.WithValue(req.Context(), contextStartTimeKey, time.Now()))

		return nil
	}
}

func newRestyLogResponseMiddleware(logger *slog.Logger) resty.ResponseMiddleware {
	return func(client *resty.Client, resp *resty.Response) error {
		// Retrieve the logger and start time from context
		reqLogger, _ := resp.Request.Context().Value(contextLoggerKey).(*slog.Logger)
		startTime, _ := resp.Request.Context().Value(contextStartTimeKey).(time.Time)

		if reqLogger == nil {
			reqLogger = logger
		}
		reqLogger = reqLogger.With(
			slog.Int("status", resp.StatusCode()),
			slog.Duration("duration", time.Since(startTime)),
			slog.Int64("content_length", resp.Size()),
		)
		if resp.IsError() {
			reqLogger.Error("error response")
		} else {
			reqLogger.Debug("response")
		}
		return nil
	}
}
