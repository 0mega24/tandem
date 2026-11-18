package gen

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = map[string]Generator{}
)

// Register adds a generator. Each generator package calls it in init(), like a database/sql
// driver. A duplicate or empty ID panics, so the first test run finds it.
func Register(g Generator) {
	id := g.Meta().ID
	if id == "" || strings.ContainsAny(id, " ,/") {
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

	out := make([]Generator, 0, len(registry))
	for _, g := range registry {
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b Generator) int { return strings.Compare(a.Meta().ID, b.Meta().ID) })
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
	all := All()
	ids := make([]string, len(all))
	for i, g := range all {
		ids[i] = g.Meta().ID
	}
	return ids
}

// Select returns the IDs of the generators to use: the defaults for nil, none for an empty list
// or ["none"], or the given ones. An unknown ID is an error that lists the known ones.
func Select(enabled []string) ([]string, error) {
	if enabled == nil {
		var ids []string
		for _, g := range All() {
			if g.Meta().DefaultOn {
				ids = append(ids, g.Meta().ID)
			}
		}
		return ids, nil
	}
	if len(enabled) == 0 || (len(enabled) == 1 && enabled[0] == "none") {
		return []string{}, nil
	}

	var ids []string
	for _, id := range enabled {
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
