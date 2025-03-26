package flagsmith

import (
	"net/http"
)

type APIError struct {
	Err      error
	response *http.Response
}

func (e APIError) Error() string {
	return e.Err.Error()
}

func (e APIError) Response() *http.Response {
	return e.response
}
