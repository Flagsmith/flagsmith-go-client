package flaghttp

type Response interface {
	Body() []byte
	IsSuccess() bool
	Status() string
	StatusCode() int
}

type response struct {
	body       []byte
	statusCode int
	status     string
}

func (r *response) Body() []byte {
	return r.body
}

func (r *response) IsSuccess() bool {
	return r.statusCode >= 200 && r.statusCode < 300
}

func (r *response) Status() string {
	return r.status
}

func (r *response) StatusCode() int {
	return r.statusCode
}
