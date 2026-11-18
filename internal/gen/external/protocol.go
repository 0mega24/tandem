package external

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

type metadata struct {
	Protocol     *int           `json:"protocol"`
	ID           *string        `json:"id"`
	Languages    *stringList    `json:"languages"`
	Manifests    *stringList    `json:"manifests"`
	DefaultOn    *bool          `json:"default_on"`
	ConfigSchema map[string]any `json:"config_schema"`
}

func (m metadata) validate(id string) error {
	if m.Protocol == nil || m.ID == nil || m.Languages == nil || m.Manifests == nil || m.DefaultOn == nil || m.ConfigSchema == nil {
		return errors.New("meta: required fields must be present and non-null")
	}
	if *m.Protocol != 1 {
		return fmt.Errorf("unknown protocol %d", *m.Protocol)
	}
	if *m.ID != id {
		return fmt.Errorf("meta ID %q does not match executable ID %q", *m.ID, id)
	}
	return nil
}

type spaceRequest struct {
	Name   string `json:"name"`
	Root   string `json:"root"`
	Branch string `json:"branch"`
}

type memberRequest struct {
	Name     string `json:"name"`
	Worktree string `json:"worktree"`
	Origin   string `json:"origin"`
}

type planRequest struct {
	Space   spaceRequest    `json:"space"`
	Members []memberRequest `json:"members"`
	Config  map[string]any  `json:"config"`
}

type fileAnswer struct {
	Path    *string `json:"path"`
	Content *string `json:"content"`
}

type linkAnswer struct {
	Path   *string `json:"path"`
	Target *string `json:"target"`
}

type planAnswer struct {
	Files    *[]fileAnswer `json:"files"`
	Stale    *stringList   `json:"stale"`
	Links    *[]linkAnswer `json:"links"`
	Warnings *stringList   `json:"warnings"`
}

func (a planAnswer) plan(root string, members []string) (gen.Plan, error) {
	if a.Files == nil || a.Stale == nil || a.Links == nil || a.Warnings == nil {
		return gen.Plan{}, errors.New("plan: required arrays must be present and non-null")
	}
	p := gen.Plan{Stale: []string(*a.Stale), Warnings: []string(*a.Warnings)}
	seen := map[string]bool{}
	for _, f := range *a.Files {
		if f.Path == nil || f.Content == nil {
			return gen.Plan{}, errors.New("file: path and content are required strings")
		}
		if err := gen.CheckPath(root, *f.Path, members); err != nil {
			return gen.Plan{}, err
		}
		if seen[*f.Path] {
			return gen.Plan{}, fmt.Errorf("path %q is planned twice", *f.Path)
		}
		seen[*f.Path] = true
		p.Files = append(p.Files, gen.File{Path: *f.Path, Content: []byte(*f.Content)})
	}
	for _, s := range p.Stale {
		if err := gen.CheckPath(root, s, members); err != nil {
			return gen.Plan{}, err
		}
	}
	for _, l := range *a.Links {
		if l.Path == nil || l.Target == nil {
			return gen.Plan{}, errors.New("link: path and target are required strings")
		}
		link := gen.Link{Path: *l.Path, Target: *l.Target}
		if err := checkLink(root, link, members); err != nil {
			return gen.Plan{}, err
		}
		if seen[*l.Path] {
			return gen.Plan{}, fmt.Errorf("path %q is planned twice", *l.Path)
		}
		seen[*l.Path] = true
		p.Links = append(p.Links, link)
	}
	for _, s := range p.Stale {
		if seen[s] {
			return gen.Plan{}, fmt.Errorf("stale path %q is also planned", s)
		}
	}
	slices.SortFunc(p.Files, func(a, b gen.File) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(p.Links, func(a, b gen.Link) int { return strings.Compare(a.Path, b.Path) })
	return p, nil
}

// checkLink allows only the Meson shape: a direct <member>/subprojects/<name> entry whose target,
// relative to the link as for built-in plans, is another member. gen.CheckLink then applies the
// rules that every link must pass.
func checkLink(root string, l gen.Link, members []string) error {
	to := path.Join(path.Dir(l.Path), l.Target)
	for _, member := range members {
		name, ok := strings.CutPrefix(l.Path, member+"/subprojects/")
		if !ok {
			continue
		}
		if name == "" || strings.ContainsAny(name, "/\\") {
			break
		}
		if to == member || !slices.Contains(members, to) {
			return fmt.Errorf("link %q: target %q must be another member worktree", l.Path, l.Target)
		}
		return gen.CheckLink(root, l, members)
	}
	return fmt.Errorf("link %q must be a direct subprojects entry of a member", l.Path)
}

// stringList rejects null elements, which encoding/json otherwise turns into empty strings.
type stringList []string

func (s *stringList) UnmarshalJSON(data []byte) error {
	var values []*string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		return errors.New("required array must not be null")
	}
	*s = make([]string, len(values))
	for i, value := range values {
		if value == nil {
			return errors.New("array elements must be strings, not null")
		}
		(*s)[i] = *value
	}
	return nil
}

// JSON object keys are case-sensitive. The Go decoder alone also accepts capitalized names.
func checkAnswer(data []byte, verb string) error {
	fields := []string{"protocol", "id", "languages", "manifests", "default_on", "config_schema"}
	if verb == "plan" {
		fields = []string{"files", "stale", "links", "warnings"}
	}
	object, err := checkObject(data, fields)
	if err != nil {
		return err
	}
	if verb != "plan" {
		return nil
	}
	for _, key := range []string{"files", "links"} {
		var entries []json.RawMessage
		if err := json.Unmarshal(object[key], &entries); err != nil {
			return err
		}
		fields := []string{"path", "content"}
		if key == "links" {
			fields = []string{"path", "target"}
		}
		for _, entry := range entries {
			if _, err := checkObject(entry, fields); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkObject(data []byte, fields []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("required JSON object must not be null")
	}
	for key := range object {
		if !slices.Contains(fields, key) {
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}
	return object, nil
}
