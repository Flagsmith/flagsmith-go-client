package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Flagsmith/flagsmith-go-client/flagengine/utils"
)

func TestGetHashedPercentageForObjectIds(t *testing.T) {
	cases := []struct {
		name       string
		ids        []string
		iterations int
		expected   float64
	}{
		{
			name:       "foobar",
			ids:        []string{"foo", "bar"},
			iterations: 3,
			expected:   85.37707541508301, // this was generated using REPL and flagsmith-engine code
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := utils.GetHashedPercentageForObjectIds(c.ids, c.iterations)
			assert.InEpsilon(t, c.expected, actual, 1e-6)
		})
	}
}
