package flagsmith

import (
	"context"
	"fmt"
	"log/slog"
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

func newRestyLogRequestMiddleware(logger *slog.Logger) resty.RequestMiddleware {
	return func(c *resty.Client, req *resty.Request) error {
		// Create a child logger with request metadata
		reqLogger := logger.WithGroup("http").With(
			"method", req.Method,
			"url", req.URL,
		)
		reqLogger.Debug("request")

		// Store the logger in this request's context, and use it in the response
		req.SetContext(context.WithValue(req.Context(), "logger", reqLogger))

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
