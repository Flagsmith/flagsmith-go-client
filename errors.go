package flagsmith

type FlagsmithClientError struct {
	msg string
}

type FlagsmithAPIError struct {
	msg string
}

func (e FlagsmithClientError) Error() string {
	return e.msg
}

func (e FlagsmithAPIError) Error() string {
	return e.msg
}
