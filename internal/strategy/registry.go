package strategy

import (
	"fmt"
	"sync"

	"github.com/allfitness/ai-gateway/internal/domain"
	"github.com/allfitness/ai-gateway/internal/prompt"
)

// Registry manages and routes strategies for different request types.
type Registry struct {
	mu         sync.RWMutex
	strategies map[domain.RequestType]Strategy
}

// NewRegistry initializes and registers all standard strategies.
func NewRegistry(tm *prompt.TemplateManager) *Registry {
	r := &Registry{
		strategies: make(map[domain.RequestType]Strategy),
	}

	r.Register(NewWorkoutStrategy(tm))
	r.Register(NewRecipeStrategy(tm))
	r.Register(NewExerciseStrategy(tm))

	return r
}

// Register adds or replaces a strategy in the registry.
func (r *Registry) Register(s Strategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategies[s.RequestType()] = s
}

// Get retrieves the strategy for the given request type.
func (r *Registry) Get(rt domain.RequestType) (Strategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.strategies[rt]
	if !ok {
		return nil, fmt.Errorf("%w: '%s'", domain.ErrUnsupportedRequestType, rt)
	}
	return s, nil
}
