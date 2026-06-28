// Package services connects Pylon to external services (calendar, GitHub, ...).
// Each Service contributes actions to the intent vocabulary and handles them; a
// Registry collects the services, exposes their action specs to the LLM, and
// dispatches resolved commands to the owning service.
package services

import (
	"context"
	"fmt"

	"github.com/YCistak/pylon/internal/intent"
)

// Service is one external integration. It declares the actions it handles (fed
// into the LLM's action catalog) and executes them, returning a user-facing,
// speakable reply.
type Service interface {
	Name() string
	Actions() []intent.ActionSpec
	Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error)
}

// Registry holds the enabled services and routes commands to them.
type Registry struct {
	services []Service
	byAction map[intent.Action]Service
}

// NewRegistry builds a Registry from the enabled services (nil entries skipped).
func NewRegistry(svcs ...Service) *Registry {
	r := &Registry{byAction: map[intent.Action]Service{}}
	for _, s := range svcs {
		if s == nil {
			continue
		}
		r.services = append(r.services, s)
		for _, spec := range s.Actions() {
			r.byAction[spec.Name] = s
		}
	}
	return r
}

// Specs returns every service's ActionSpecs, to pass to intent.SetActions so the
// LLM knows the service actions.
func (r *Registry) Specs() []intent.ActionSpec {
	var out []intent.ActionSpec
	for _, s := range r.services {
		out = append(out, s.Actions()...)
	}
	return out
}

// Dispatch routes a resolved Command to its owning service. ok is false when no
// service owns the action, so the caller can fall back to built-in handling.
func (r *Registry) Dispatch(ctx context.Context, cmd intent.Command) (text string, ok bool, err error) {
	s, found := r.byAction[cmd.Action]
	if !found {
		return "", false, nil
	}
	out, err := s.Execute(ctx, cmd.Action, cmd.Args)
	if err != nil {
		return "", true, fmt.Errorf("%s: %w", s.Name(), err)
	}
	return out, true, nil
}
