package utils

import (
	"strings"
	"time"

	"github.com/itlightning/dateparse"
)

type ISOTime struct {
	time.Time
}

func (i *ISOTime) UnmarshalJSON(bytes []byte) (err error) {
	i.Time, err = dateparse.ParseAny(strings.Trim(string(bytes), `"`))
	return
}

func (i *ISOTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + i.Time.Format(time.RFC3339) + `"`), nil
}
