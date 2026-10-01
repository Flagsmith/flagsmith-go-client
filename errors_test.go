package flagsmith_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	flagsmith "github.com/Flagsmith/flagsmith-go-client/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestErrorChain(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if want == context.DeadlineExceeded {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
			}
			defer cancel()
			cancel()
			client := flagsmith.NewClient("env-key", flagsmith.WithBaseURL("http://127.0.0.1:1/"))
			methods := map[string]func() error{
				"environment API": func() error { _, err := client.GetEnvironmentFlagsFromAPI(ctx); return err },
				"identity API":    func() error { _, err := client.GetIdentityFlagsFromAPI(ctx, "id", nil); return err },
				"environment":     func() error { _, err := client.GetEnvironmentFlags(ctx); return err },
				"identity":        func() error { _, err := client.GetIdentityFlags(ctx, "id", nil); return err },
			}
			for name, call := range methods {
				t.Run(name, func(t *testing.T) {
					err := call()
					require.ErrorIs(t, err, want)
					var apiErr *flagsmith.FlagsmithAPIError
					require.ErrorAs(t, err, &apiErr)
					var transportErr *url.Error
					assert.ErrorAs(t, err, &transportErr)
				})
			}
		})
	}
}

func TestAPIErrorWithoutCause(t *testing.T) {
	for _, err := range []error{flagsmith.FlagsmithAPIError{Msg: "unchanged"}, &flagsmith.FlagsmithAPIError{Msg: "unchanged"}} {
		assert.Equal(t, "unchanged", err.Error())
		assert.Nil(t, errors.Unwrap(err))
		assert.False(t, errors.Is(err, context.Canceled))
	}
}

func TestAPIErrorUnwrap(t *testing.T) {
	for _, err := range []error{flagsmith.FlagsmithAPIError{Msg: "unchanged", Err: context.Canceled}, &flagsmith.FlagsmithAPIError{Msg: "unchanged", Err: context.Canceled}} {
		assert.Equal(t, "unchanged", err.Error())
		assert.Same(t, context.Canceled, errors.Unwrap(err))
		assert.ErrorIs(t, err, context.Canceled)
	}
}
