package gen

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = map[string]Generator{}

	// idPattern keeps an ID usable as a TOML and YAML key, a CLI value and a plugin name.
	idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Register adds a generator. Each generator package calls it in init(), like a database/sql
// driver. A duplicate or empty ID panics, so the first test run finds it.
func Register(g Generator) { RegisterID(g.Meta().ID, g) }

// RegisterID adds a generator under id without a call to Meta, so a plugin runs its meta only
// when a command needs it.
func RegisterID(id string, g Generator) {
	if !idPattern.MatchString(id) || id == "none" {
		panic(fmt.Sprintf("gen: invalid generator ID %q", id))
	}

	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[id]; dup {
		panic("gen: generator " + id + " registered twice")
	}
	registry[id] = g
}

// All returns the generators sorted by ID, so the output and the docs have a fixed order.
func All() []Generator {
	mu.RLock()
	defer mu.RUnlock()

	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]Generator, len(ids))
	for i, id := range ids {
		out[i] = registry[id]
	}
	return out
}

// ByID returns the generator with the ID.
func ByID(id string) (Generator, bool) {
	mu.RLock()
	defer mu.RUnlock()
	g, ok := registry[id]
	return g, ok
}

// IDs returns the IDs of all generators, sorted.
func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Select returns the IDs of the generators to use: the defaults for nil, none for an empty list
// or ["none"], or the given ones. An unknown ID is an error that lists the known ones.
func Select(enabled []string) ([]string, error) {
	if enabled == nil {
		var ids []string
		for _, id := range IDs() {
			g, _ := ByID(id)
			if g.Meta().DefaultOn {
				ids = append(ids, id)
			}
		}
		return ids, nil
	}
	if len(enabled) == 0 || (len(enabled) == 1 && enabled[0] == "none") {
		return []string{}, nil
	}

	var ids []string
	for _, id := range enabled {
		if id == "none" {
			return nil, errors.New(`generate.enabled: "none" cannot be combined with generator IDs`)
		}
		if _, ok := ByID(id); !ok {
			return nil, fmt.Errorf("unknown generator %q; known: %s (or none)", id, strings.Join(IDs(), ", "))
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// CheckConfig checks the [generate] settings against the registry: the enabled IDs and the
// names of the [generate.<id>] tables, so a typo such as [generate.rusty] is an error.
func CheckConfig(enabled []string, tables map[string]any) error {
	if enabled != nil { // the default set needs the meta of each plugin, and it cannot be wrong
		if _, err := Select(enabled); err != nil {
			return err
		}
	}
	for id := range tables {
		if _, ok := ByID(id); !ok {
			return fmt.Errorf("[generate.%s]: unknown generator; known: %s", id, strings.Join(IDs(), ", "))
		}
	}
	return nil
}

// Unregister removes a temporary generator after an embedded CLI invocation finishes.
// Built-in registrations last for the process lifetime and must not be removed.
func Unregister(id string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, id)
}
