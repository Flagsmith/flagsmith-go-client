package flagsmith

import (
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/environments"
	"github.com/Flagsmith/flagsmith-go-client/v4/flagengine/identities"
	"sync"
)

// environmentState is a locally cached environments.EnvironmentModel.
type environmentState struct {
	environment *environments.EnvironmentModel
	offline     bool
	mu          sync.RWMutex

	identityOverrides sync.Map
}

// GetEnvironment returns the current environment and indicates if it was initialised.
func (cs *environmentState) GetEnvironment() (*environments.EnvironmentModel, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.environment == nil {
		return nil, false
	}
	return cs.environment, true
}

func (cs *environmentState) GetIdentityOverride(identifier string) (*identities.IdentityModel, bool) {
	i, ok := cs.identityOverrides.Load(identifier)
	if ok && i != nil {
		return i.(*identities.IdentityModel), true
	}
	return nil, false
}

func (cs *environmentState) SetEnvironment(env *environments.EnvironmentModel) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.environment = env

	// clear previous overrides before storing the new ones
	cs.identityOverrides = sync.Map{}
	for _, id := range env.IdentityOverrides {
		cs.identityOverrides.Store(id.Identifier, id)
	}
}

func (cs *environmentState) SetOfflineEnvironment(env *environments.EnvironmentModel) {
	cs.SetEnvironment(env)
	cs.offline = true
}

func (cs *environmentState) IsOffline() bool {
	return cs.offline
}
