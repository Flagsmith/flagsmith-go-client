package flagsmith

import (
	"encoding/json"
	"os"

	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
)

type OfflineHandler interface {
	GetEnvironment() *environments.EnvironmentModel
}

type LocalFileHandler struct {
	environment *environments.EnvironmentModel
}

// NewLocalFileHandler creates a new LocalFileHandler with the given path.
func NewLocalFileHandler(environmentDocumentPath string) (*LocalFileHandler, error) {
	// Read the environment document from the specified path
	environmentDocument, err := os.ReadFile(environmentDocumentPath)
	if err != nil {
		return nil, err
	}
	var environment environments.EnvironmentModel
	if err := json.Unmarshal(environmentDocument, &environment); err != nil {
		return nil, err
	}

	// Create and initialize the LocalFileHandler
	handler := &LocalFileHandler{
		environment: &environment,
	}

	return handler, nil
}

func (handler *LocalFileHandler) GetEnvironment() *environments.EnvironmentModel {
	return handler.environment
}
