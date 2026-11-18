package vscode

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

// Generator writes the editor workspace file.
type Generator struct{}

// Key is the one key of the file that tdm owns; the person keeps the others, such as "settings".
const Key = "folders"

func (Generator) Meta() gen.Meta {
	return gen.Meta{ID: "vscode", Languages: []string{"All"}, Manifests: []string{"any member"}, Release: 2}
}

// Detect accepts every member: each one is a folder of the workspace.
func (Generator) Detect(m gen.Member, _ gen.DetectOptions) (*gen.Detection, error) {
	return &gen.Detection{Member: m}, nil
}

type folder struct {
	Path string `json:"path"`
}

func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	if len(found) == 0 {
		return gen.Plan{}, nil
	}
	dirs := make([]gen.Member, 0, len(found)+len(s.Symlinks))
	for _, d := range found {
		dirs = append(dirs, d.Member)
	}
	dirs = append(dirs, s.Symlinks...)
	slices.SortFunc(dirs, func(a, b gen.Member) int { return strings.Compare(a.Name, b.Name) })

	folders := make([]folder, 0, len(dirs))
	for _, m := range dirs {
		rel, err := filepath.Rel(s.Root, m.WorktreePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return gen.Plan{}, fmt.Errorf("%s is not inside the space root", m.Name)
		}
		folders = append(folders, folder{Path: filepath.ToSlash(rel)})
	}

	b, err := json.MarshalIndent(map[string][]folder{Key: folders}, "", "\t")
	if err != nil {
		return gen.Plan{}, err
	}
	return gen.Plan{Files: []gen.File{{Path: s.Name + ".code-workspace", Content: append(b, '\n'), Key: Key}}}, nil
}
