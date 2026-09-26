package module

import "sync"

// Registry is a typed service locator between modules. A module that offers
// something to others calls Provide in New; consumers call Lookup in Start
// (after every module was built).
//
//	module.Provide[habits.Checkin](d.Registry, "habits.checkin", svc)
//	checkin, ok := module.Lookup[habits.Checkin](d.Registry, "habits.checkin")
type Registry struct {
	mu    sync.RWMutex
	items map[string]any
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{items: map[string]any{}} }

// Provide stores v under name.
func Provide[T any](r *Registry, name string, v T) {
	r.mu.Lock()
	r.items[name] = v
	r.mu.Unlock()
}

// Lookup returns the value stored under name if it has type T.
func Lookup[T any](r *Registry, name string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[name].(T)
	return v, ok
}

// All returns every value whose type is T, for example all automation actions.
func All[T any](r *Registry) map[string]T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]T{}
	for k, v := range r.items {
		if t, ok := v.(T); ok {
			out[k] = t
		}
	}
	return out
}
