package flaghttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
)

type Request interface {
	ForceContentType(contentType string) Request
	Get(url string) (Response, error)
	Post(url string) (Response, error)
	SetBody(body any) Request
	SetContext(ctx context.Context) Request
	SetResult(res any) Request
	SetError(err any) Request
}

type request struct {
	client      *client
	body        any
	ctx         context.Context
	err         any
	result      any
	contentType string
	retryCount  int
}

func (r *request) ForceContentType(contentType string) Request {
	r.contentType = contentType

	return r
}

func (r *request) Get(url string) (Response, error) {
	return r.do(http.MethodGet, url)
}

func (r *request) Post(url string) (Response, error) {
	return r.do(http.MethodPost, url)
}

func (r *request) SetBody(body any) Request {
	r.body = body

	return r
}

func (r *request) SetContext(ctx context.Context) Request {
	r.ctx = ctx

	return r
}

func (r *request) SetResult(res any) Request {
	r.result = res

	return r
}

func (r *request) SetError(err any) Request {
	r.err = err

	return r
}

func (r *request) do(method, url string) (Response, error) {
	var (
		transport = r.client.transport.Clone()
		client    = &http.Client{
			Transport: transport,
			Timeout:   transport.ResponseHeaderTimeout,
		}
		req *http.Request
		err error
		b   io.Reader
	)

	if r.body != nil {
		var buf bytes.Buffer

		if err := json.NewEncoder(&buf).Encode(r.body); err != nil {
			return &response{
				statusCode: http.StatusInternalServerError,
				status:     http.StatusText(http.StatusInternalServerError),
			}, err
		}

		// Trim the last newline character.
		buf.Truncate(buf.Len() - 1)
		b = &buf
	} else {
		b = http.NoBody
	}

	if r.ctx != nil {
		req, err = http.NewRequestWithContext(r.ctx, method, url, b)
	} else {
		req, err = http.NewRequest(method, url, b)
	}

	if err != nil {
		return &response{
			statusCode: http.StatusInternalServerError,
			status:     http.StatusText(http.StatusInternalServerError),
		}, err
	}

	for k, v := range r.client.header {
		req.Header[k] = v
	}

	res, err := client.Do(req)
	if err != nil {
		if r.client.retryCount > 0 && r.retryCount < r.client.retryCount {
			r.retryCount++

			if r.client.retryWait > 0 {
				time.Sleep(r.client.retryWait)
			}

			return r.do(method, url)
		}

		return &response{
			statusCode: http.StatusInternalServerError,
			status:     http.StatusText(http.StatusInternalServerError),
		}, err
	}

	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return &response{
			statusCode: http.StatusInternalServerError,
			status:     http.StatusText(http.StatusInternalServerError),
		}, err
	}

	if res.StatusCode >= 400 {
		if r.err != nil {
			if err := json.Unmarshal(bodyBytes, r.err); err != nil {
				return &response{
					body:       bodyBytes,
					statusCode: res.StatusCode,
					status:     res.Status,
				}, err
			}
		}

		return &response{
			body:       bodyBytes,
			statusCode: res.StatusCode,
			status:     res.Status,
		}, errors.New(res.Status)
	}

	if r.contentType != "" {
		gotContentType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
		if err != nil {
			return &response{
				body:       bodyBytes,
				statusCode: res.StatusCode,
				status:     res.Status,
			}, err
		}

		wantContentType, _, err := mime.ParseMediaType(r.contentType)
		if err != nil {
			return &response{
				body:       bodyBytes,
				statusCode: res.StatusCode,
				status:     res.Status,
			}, err
		}

		if gotContentType != wantContentType {
			return &response{
				body:       bodyBytes,
				statusCode: res.StatusCode,
				status:     res.Status,
			}, errors.New("unexpected content type")
		}
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if r.result != nil {
			if err := json.Unmarshal(bodyBytes, r.result); err != nil {
				return &response{
					body:       bodyBytes,
					statusCode: res.StatusCode,
					status:     res.Status,
				}, err
			}
		}
	}

	return &response{
		body:       bodyBytes,
		statusCode: res.StatusCode,
		status:     res.Status,
	}, nil
}
