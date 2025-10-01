package utils_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/v5/flagengine/utils"
)

func TestUnmarshal(t *testing.T) {
	// few cases from engine-test-data
	cases := []struct {
		input    string
		expected time.Time
	}{
		{
			`"2021-12-15T14:40:00.881386"`,
			time.Date(2021, 12, 15, 14, 40, 0, 881386000, time.UTC),
		},
		{
			`"2021-12-15T14:40:00.881398"`,
			time.Date(2021, 12, 15, 14, 40, 0, 881398000, time.UTC),
		},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			assert := assert.New(t)
			var actual utils.ISOTime
			err := actual.UnmarshalJSON([]byte(c.input))
			assert.NoError(err)
			assert.Equal(c.expected, actual.Time)
		})
	}
}
