package flagsmith

type FlagsmithClientError struct {
	msg string
	err error
}

type FlagsmithAPIError struct {
	Msg                string
	Err                error
	ResponseStatusCode int
	ResponseStatus     string
}

func (e FlagsmithClientError) Error() string {
	return e.msg
}

func (e FlagsmithAPIError) Error() string {
	return e.Msg
}

// Unwrap returns the underlying error.
func (e FlagsmithClientError) Unwrap() error {
	return e.err
}

// Unwrap returns the underlying error.
func (e FlagsmithAPIError) Unwrap() error {
	return e.Err
}
