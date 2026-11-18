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
	"time"

	"github.com/0mega24/tandem/internal/gen"
)

const (
	timeLimit   = 60 * time.Second
	outputLimit = 16 << 20
	supported   = "supported versions: 1"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Plugin caches meta and runs plan on demand. Stderr defaults to os.Stderr.
type Plugin struct {
	Path    string
	Stderr  io.Writer
	meta    gen.Meta
	timeout time.Duration
}

var _ gen.Generator = Plugin{}

// Discover searches dirs, the XDG user plugin directory, then path. The first executable
// for each ID wins. Each selected executable runs meta exactly once.
func Discover(dirs []string, path string) ([]Plugin, error) {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("plugin home: %w", err)
		}
		data = filepath.Join(home, ".local", "share")
	}
	search := append([]string{}, dirs...)
	search = append(search, filepath.Join(data, "tandem", "plugins"))
	if path != "" {
		search = append(search, filepath.SplitList(path)...)
	}
	seen := map[string]bool{}
	var plugins []Plugin
	for _, dir := range search {
		if dir == "" {
			dir = "."
		}
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("plugin directory %s: %w", dir, err)
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
				return nil, fmt.Errorf("plugin %s: %w", file, err)
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
				continue
			}
			absolute, err := filepath.Abs(file)
			if err != nil {
				return nil, err
			}
			p := Plugin{Path: absolute, timeout: timeLimit, meta: gen.Meta{ID: id}}
			var meta metadata
			if err := p.run("meta", nil, &meta); err != nil {
				return nil, err
			}
			if err := meta.validate(id); err != nil {
				return nil, p.failure("meta", err)
			}
			p.meta = gen.Meta{ID: id, Languages: []string(*meta.Languages), Manifests: []string(*meta.Manifests), DefaultOn: *meta.DefaultOn}
			plugins = append(plugins, p)
			seen[id] = true
		}
	}
	return plugins, nil
}

func (p Plugin) Meta() gen.Meta { return p.meta }

// Detect includes every member; the plugin decides which manifests apply during plan.
func (Plugin) Detect(m gen.Member, _ gen.DetectOptions) (*gen.Detection, error) {
	return &gen.Detection{Member: m}, nil
}

// Plan is the exception to the pure generator API: it runs the plugin process. Only
// Tandem's apply layer writes the returned files; plugins must only read member files.
func (p Plugin) Plan(s gen.Space, _ []gen.Detection) (gen.Plan, error) {
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
	return fmt.Errorf("plugin %s %s (%s): %w", p.meta.ID, verb, supported, err)
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
	cmd.Stderr = &prefixWriter{out: stderr, prefix: p.meta.ID + ": ", start: true}
	// A descendant must not keep the stdout/stderr pipes open after the process exits.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
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
