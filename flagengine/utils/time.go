package utils

import (
	"strings"
	"time"
)

const iso8601 = "2006-01-02T15:04:05.999999"

type ISOTime struct {
	time.Time
}

func (i *ISOTime) UnmarshalJSON(bytes []byte) (err error) {
	i.Time, err = time.Parse(iso8601, strings.Trim(string(bytes), `"`))
	return
}

func (i *ISOTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + i.Time.Format(iso8601) + `"`), nil
}
