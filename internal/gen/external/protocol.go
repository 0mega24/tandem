package external

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
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
		if err := checkLink(root, *l.Path, *l.Target, members); err != nil {
			return gen.Plan{}, err
		}
		if seen[*l.Path] {
			return gen.Plan{}, fmt.Errorf("path %q is planned twice", *l.Path)
		}
		seen[*l.Path] = true
		p.Links = append(p.Links, gen.Link{Path: *l.Path, Target: *l.Target})
	}
	return p, nil
}

// checkLink allows only a direct subprojects entry in one member pointing to another member.
func checkLink(root, p, target string, members []string) error {
	if !filepath.IsAbs(target) {
		return errors.New("link target must be absolute")
	}
	target = filepath.Clean(target)
	targetMember := ""
	for _, member := range members {
		if filepath.Join(root, filepath.FromSlash(member)) == target {
			targetMember = member
			break
		}
	}
	if targetMember == "" {
		return errors.New("link target must be a member worktree")
	}
	for _, member := range members {
		prefix := member + "/subprojects/"
		if strings.HasPrefix(p, prefix) && member != targetMember {
			name := strings.TrimPrefix(p, prefix)
			if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
				break
			}
			// CheckPath checks all existing path components for symlinks. It must allow this
			// one member destination, so omit the member prohibition for the link path only.
			if path.Clean(p) != p {
				break
			}
			return gen.CheckPath(root, p, nil)
		}
	}
	return errors.New("link path must be a member's direct subprojects entry")
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
