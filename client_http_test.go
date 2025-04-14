//go:build test

package flagsmith

import (
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
)

func (c *Client) ExposeRestyClient() *resty.Client {
	return c.client
}

func TestCustomClientRetriesAreSet(t *testing.T) {
	retryCount := 5

	customResty := resty.New().
		SetRetryCount(retryCount).
		SetRetryWaitTime(10 * time.Millisecond)

	client := NewClient("env-key", WithRestyClient(customResty))

	internal := client.ExposeRestyClient()
	assert.Equal(t, retryCount, internal.RetryCount)
	assert.Equal(t, 10*time.Millisecond, internal.RetryWaitTime)
}

func TestCustomRestyClientTimeoutIsNotOverriddenWithDefaultTimeout(t *testing.T) {
	customResty := resty.New().SetTimeout(13 * time.Millisecond)

	client := NewClient("env-key", WithRestyClient(customResty))

	internal := client.ExposeRestyClient()

	assert.Equal(t, 13*time.Millisecond, internal.GetClient().Timeout)
}

func TestCustomRestyClientHasDefaultTimeoutIfNotProvided(t *testing.T) {
	customResty := resty.New()

	client := NewClient("env-key", WithRestyClient(customResty))

	internal := client.ExposeRestyClient()
	assert.Equal(t, 10*time.Second, internal.GetClient().Timeout)
}
