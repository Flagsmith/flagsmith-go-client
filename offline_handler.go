package flagsmith

import (
	"encoding/json"
	"os"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
)

type Environment interface {
	GetEnvironment() *environments.EnvironmentModel
}

type environment struct {
	model *environments.EnvironmentModel
}

func (e environment) GetEnvironment() *environments.EnvironmentModel {
	return e.model
}

// ReadEnvironmentFromFile reads an Environment from a file path.
func ReadEnvironmentFromFile(name string) (env Environment, err error) {
	file, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var model environments.EnvironmentModel
	err = json.Unmarshal(file, &model)
	env = environment{model: &model}
	return
}
