package space

import (
	"io"
	"sync"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/state"
)

// Manager runs the operations with one config, one git and one state store.
type Manager struct {
	cfg     config.Config
	git     git.Runner
	store   state.Store
	hookOut io.Writer // nil drops the output of the always.run hook
}

// New returns a Manager. Tests pass a fake git.Runner to make one step fail.
func New(cfg config.Config, g git.Runner, store state.Store) *Manager {
	return &Manager{cfg: cfg, git: g, store: store}
}

// maxParallel bounds the git processes that run at once; fetches are network-bound, status is
// disk-bound, and more than this only adds contention.
const maxParallel = 8

// parallel runs fn for 0..n-1 with at most maxParallel at once, and waits for all.
func parallel(n int, fn func(i int)) {
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i := range n {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			fn(i)
		})
	}

	wg.Wait()
}
