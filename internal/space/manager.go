package space

import (
	"fmt"
	"io"
	"sync"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/git"
	"github.com/0mega24/tandem/internal/state"
)

// Manager runs the operations with one config, one git and one state store.
type Manager struct {
	cfg   config.Config
	git   git.Runner
	store state.Store
	out   io.Writer // the always.run output and the warnings; nil drops them
}

// New returns a Manager. Tests pass a fake git.Runner to make one step fail.
func New(cfg config.Config, g git.Runner, store state.Store) *Manager {
	return &Manager{cfg: cfg, git: g, store: store}
}

// SetOutput sets where the always.run hook writes and where warnings go. The operations do not
// fail on these, so the output is the only place to report them. Without it they are dropped.
func (m *Manager) SetOutput(w io.Writer) { m.out = w }

// warn reports a problem that does not stop the operation.
func (m *Manager) warn(format string, args ...any) {
	if m.out != nil {
		_, _ = fmt.Fprintf(m.out, "tdm: warning: "+format+"\n", args...) // there is no other place to report it
	}
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
