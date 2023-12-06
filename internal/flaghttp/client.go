package flaghttp

import (
	"net"
	"net/http"
	"net/url"
	"time"
)

type Logger interface {
	Errorf(format string, v ...any)
	Warnf(format string, v ...any)
	Debugf(format string, v ...any)
}

type Client interface {
	NewRequest() Request
	R() Request
	SetHeader(header, value string) Client
	SetHeaders(headers map[string]string) Client
	SetLogger(l Logger) Client
	SetProxy(proxyURL string) Client
	SetRetryCount(count int) Client
	SetRetryWaitTime(waitTime time.Duration) Client
	SetTimeout(timeout time.Duration) Client
}

type client struct {
	logger     Logger
	retryCount int
	retryWait  time.Duration
	header     http.Header
	transport  *http.Transport
}

func NewClient() Client {
	return &client{
		header: http.Header{},
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func (c *client) NewRequest() Request {
	return c.R()
}

func (c *client) R() Request {
	return &request{
		client: c,
	}
}

func (c *client) SetHeader(header, value string) Client {
	c.header.Set(header, value)

	return c
}

func (c *client) SetHeaders(headers map[string]string) Client {
	for k, v := range headers {
		c.header.Set(k, v)
	}

	return c
}

func (c *client) SetLogger(l Logger) Client {
	c.logger = l

	return c
}

func (c *client) SetProxy(proxyURL string) Client {
	if proxyURL == "" {
		return c
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		c.logger.Warnf("Failed to parse proxy URL: %s", err)
		return c
	}

	c.transport.Proxy = http.ProxyURL(u)

	return c
}

func (c *client) SetRetryCount(count int) Client {
	c.retryCount = count

	return c
}

func (c *client) SetRetryWaitTime(waitTime time.Duration) Client {
	c.retryWait = waitTime

	return c
}

func (c *client) SetTimeout(timeout time.Duration) Client {
	c.transport.ResponseHeaderTimeout = timeout

	return c
}
