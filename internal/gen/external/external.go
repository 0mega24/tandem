// Package external adapts protocol-v1 executables to the generator API.
package external

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/0mega24/tandem/internal/gen"
)

const (
	timeLimit   = 60 * time.Second
	outputLimit = 16 << 20
	supported   = "supported versions: 1"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Plugin runs meta once, when a command first needs it, and plan on demand. Stderr defaults to
// os.Stderr. Most commands (status, path, exec) never need a plugin, so they run none.
type Plugin struct {
	Path    string
	Stderr  io.Writer
	id      string
	timeout time.Duration
	loaded  *loaded // shared by the copies of a Plugin, so meta runs once
}

type loaded struct {
	once sync.Once
	meta gen.Meta
	err  error
}

var _ gen.Generator = Plugin{}

// Discover searches dirs, the XDG user plugin directory, then path. The first executable
// for each ID wins. It runs no plugin: see Load. A plugin's stderr goes to stderr.
//
// A plugin with the ID of a built-in generator is in broken and not in plugins, so a stray
// tdm-gen-go on PATH does not block commands. err is for a plugin directory that cannot be read.
func Discover(dirs []string, path string, stderr io.Writer) (plugins []Plugin, broken []error, err error) {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, fmt.Errorf("plugin home: %w", err)
		}
		data = filepath.Join(home, ".local", "share")
	}
	search := append([]string{}, dirs...)
	search = append(search, filepath.Join(data, "tandem", "plugins"))
	if path != "" {
		search = append(search, filepath.SplitList(path)...)
	}
	seen := map[string]bool{}
	for _, dir := range search {
		if dir == "" {
			continue // POSIX reads an empty PATH entry as ".": never run a plugin from the current dir
		}
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("plugin directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			id, ok := strings.CutPrefix(entry.Name(), "tdm-gen-")
			if !ok || seen[id] || !idPattern.MatchString(id) || id == "none" {
				continue
			}
			file := filepath.Join(dir, entry.Name())
			info, err := os.Stat(file) //nolint:gosec // G703: ReadDir supplies the name; plugin directories are user-selected
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, nil, fmt.Errorf("plugin %s: %w", file, err)
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
				continue
			}
			absolute, err := filepath.Abs(file)
			if err != nil {
				return nil, nil, err
			}
			seen[id] = true // a bad first match is not replaced by a later one
			if _, ok := gen.ByID(id); ok {
				broken = append(broken, fmt.Errorf("plugin %s: the ID %q is a built-in generator", absolute, id))
				continue
			}
			plugins = append(plugins, Plugin{Path: absolute, Stderr: stderr, id: id, timeout: timeLimit, loaded: &loaded{}})
		}
	}
	return plugins, broken, nil
}

// ID is the ID from the executable name; it needs no meta.
func (p Plugin) ID() string { return p.id }

// Load runs meta the first time and returns its error after that.
func (p Plugin) Load() error {
	p.loaded.once.Do(func() {
		var meta metadata
		if err := p.run("meta", nil, &meta); err != nil {
			p.loaded.err = err
			return
		}
		if err := meta.validate(p.id); err != nil {
			p.loaded.err = p.failure("meta", err)
			return
		}
		p.loaded.meta = gen.Meta{ID: p.id, Languages: []string(*meta.Languages), Manifests: []string(*meta.Manifests), DefaultOn: *meta.DefaultOn}
	})
	return p.loaded.err
}

// Meta runs meta if no command did yet. A plugin whose meta fails is off by default; Plan and
// Load return the error.
func (p Plugin) Meta() gen.Meta {
	if p.Load() != nil {
		return gen.Meta{ID: p.id}
	}
	return p.loaded.meta
}

// Detect includes every member; the plugin decides which manifests apply during plan, as tdm
// cannot run plugin code in Detect. So the meta languages and manifests are only for the docs.
func (Plugin) Detect(m gen.Member, _ gen.DetectOptions) (*gen.Detection, error) {
	return &gen.Detection{Member: m}, nil
}

// Plan is the exception to the pure generator API: it runs the plugin process. Only
// Tandem's apply layer writes the returned files; plugins must only read member files.
func (p Plugin) Plan(s gen.Space, _ []gen.Detection) (gen.Plan, error) {
	if err := p.Load(); err != nil {
		return gen.Plan{}, err
	}
	request := planRequest{Space: spaceRequest{Name: s.Name, Root: s.Root, Branch: s.Branch}, Members: []memberRequest{}, Config: s.Config}
	if request.Config == nil {
		request.Config = map[string]any{}
	}
	members := make([]string, 0, len(s.Members))
	for _, m := range s.Members {
		request.Members = append(request.Members, memberRequest{Name: m.Name, Worktree: m.WorktreePath, Origin: m.OriginURL})
		rel, err := filepath.Rel(s.Root, m.WorktreePath)
		if err != nil {
			return gen.Plan{}, p.failure("plan", err)
		}
		members = append(members, filepath.ToSlash(rel))
	}
	input, err := json.Marshal(request)
	if err != nil {
		return gen.Plan{}, p.failure("plan", err)
	}
	var answer planAnswer
	if err := p.run("plan", input, &answer); err != nil {
		return gen.Plan{}, err
	}
	plan, err := answer.plan(s.Root, members)
	if err != nil {
		return gen.Plan{}, p.failure("plan", err)
	}
	return plan, nil
}

func (p Plugin) failure(verb string, err error) error {
	return fmt.Errorf("plugin %s %s (%s): %w", p.id, verb, supported, err)
}

func (p Plugin) run(verb string, input []byte, answer any) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Path, verb) //nolint:gosec // G204: a discovered executable chosen by the user
	cmd.Stdin = bytes.NewReader(input)
	out := limitedOutput{cancel: cancel}
	cmd.Stdout = &out
	stderr := p.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	errOut := &prefixWriter{out: stderr, prefix: p.id + ": ", start: true}
	cmd.Stderr = errOut
	// A plugin is often a script that starts other programs: the time and output limits kill its
	// whole process group, not only the script. WaitDelay closes the pipes that a child still holds.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if !errOut.start {
		_, _ = io.WriteString(stderr, "\n") // end a last line with no newline; stderr is best effort
	}
	if out.overflow {
		return p.failure(verb, errors.New("16 MiB stdout limit exceeded"))
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return p.failure(verb, errors.New("60 s time limit exceeded"))
	}
	if err != nil {
		return p.failure(verb, err)
	}
	if err := checkAnswer(out.buffer.Bytes(), verb); err != nil {
		return p.failure(verb, fmt.Errorf("invalid JSON: %w", err))
	}
	decoder := json.NewDecoder(bytes.NewReader(out.buffer.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(answer); err != nil {
		return p.failure(verb, fmt.Errorf("invalid JSON: %w", err))
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return p.failure(verb, errors.New("invalid JSON: expected exactly one object"))
	}
	return nil
}

type limitedOutput struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	overflow bool
}

func (w *limitedOutput) Write(b []byte) (int, error) {
	if len(b) > outputLimit-w.buffer.Len() {
		w.overflow = true
		w.cancel()
		return 0, errors.New("stdout limit exceeded")
	}
	return w.buffer.Write(b)
}

type prefixWriter struct {
	out    io.Writer
	prefix string
	start  bool
}

func (w *prefixWriter) Write(b []byte) (int, error) {
	written := 0
	for len(b) > 0 {
		if w.start {
			if _, err := io.WriteString(w.out, w.prefix); err != nil {
				return written, err
			}
			w.start = false
		}
		end := bytes.IndexByte(b, '\n')
		size := len(b)
		if end >= 0 {
			size = end + 1
		}
		n, err := w.out.Write(b[:size])
		written += n
		if err != nil {
			return written, err
		}
		if n != size {
			return written, io.ErrShortWrite
		}
		w.start = end >= 0
		b = b[size:]
	}
	return written, nil
}
