package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
)

// restySlogLogger implements a [resty.Logger] using a [slog.Logger].
type restySlogLogger struct {
	logger *slog.Logger
}

func (s restySlogLogger) Errorf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	s.logger.Error(msg)
}

func (s restySlogLogger) Warnf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	s.logger.Warn(msg)
}

func (s restySlogLogger) Debugf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	s.logger.Debug(msg)
}

func defaultLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, nil)).WithGroup("flagsmith")
}

func newRestyLogRequestMiddleware(logger *slog.Logger) resty.RequestMiddleware {
	return func(c *resty.Client, req *resty.Request) error {
		// Create a child logger with request metadata
		reqLogger := logger.With(
			"method", req.Method,
			"url", req.URL,
		)
		// Store the logger in this request's context, and use it in the response
		req.SetContext(context.WithValue(req.Context(), "logger", reqLogger))

		reqLogger.Debug("request",
			slog.String("method", req.Method),
			slog.String("url", req.URL),
		)

		// Time the current request
		req.SetContext(context.WithValue(req.Context(), "startTime", time.Now()))

		return nil
	}
}

func newRestyLogResponseMiddleware(logger *slog.Logger) resty.ResponseMiddleware {
	return func(client *resty.Client, resp *resty.Response) error {
		// Retrieve the logger and start time from context
		reqLogger, _ := resp.Request.Context().Value("logger").(*slog.Logger)
		startTime, _ := resp.Request.Context().Value("startTime").(time.Time)

		if reqLogger == nil {
			reqLogger = logger
		}
		attrs := []slog.Attr{
			slog.Int("status", resp.StatusCode()),
			slog.Duration("duration", time.Since(startTime)),
			slog.Int64("content_length", resp.Size()),
		}
		reqLogger.Debug("response",
			slog.Int("status", resp.StatusCode()),
			slog.Duration("duration", time.Since(startTime)),
			slog.Int64("content_length", resp.Size()),
		)
		msg := "received error response"
		level := slog.LevelDebug
		if resp.IsError() {
			level = slog.LevelError
		}
		logger.LogAttrs(context.Background(), level, msg, attrs...)

		return nil
	}
}
