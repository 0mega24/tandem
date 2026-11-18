package dotnet

import (
	"bufio"
	"bytes"
	"crypto/sha1" //nolint:gosec // G505: UUIDv5 requires SHA-1 for identifiers
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/0mega24/tandem/internal/gen"
)

func init() { gen.Register(Generator{}) }

type Generator struct{}

type Format string

const (
	FormatSLNX Format = "slnx"
	FormatSLN  Format = "sln"
)

type Config struct {
	Format Format `toml:"format" jsonschema:"enum=slnx,enum=sln,default=slnx,description=Solution file format"`
}

func (Generator) Meta() gen.Meta {
	return gen.Meta{
		ID: "dotnet", Languages: []string{"C#", "F#", "VB"},
		Manifests: []string{"*.slnx", "*.sln", "*.csproj", "*.fsproj", "*.vbproj"},
		DefaultOn: true, Release: 1, Config: Config{Format: FormatSLNX},
	}
}

func (Generator) Detect(m gen.Member, o gen.DetectOptions) (*gen.Detection, error) {
	solutions, err := gen.Find(m.WorktreePath, 0, "*.slnx", "*.sln")
	if err != nil {
		return nil, err
	}
	if len(solutions) == 0 && o.Depth > 0 {
		solutions, err = gen.Find(m.WorktreePath, o.Depth, "*.slnx", "*.sln")
		if err != nil {
			return nil, err
		}
	}
	var projects, warnings []string
	var manifest string
	if len(solutions) > 0 {
		// Prefer the current format, then the first path, when the repo is ambiguous.
		slices.SortStableFunc(solutions, func(a, b string) int {
			return strings.Compare(path.Ext(b), path.Ext(a))
		})
		manifest = solutions[0]
		projects, err = readSolution(m.WorktreePath, manifest)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", m.Name, manifest, err)
		}
		if len(solutions) > 1 {
			warnings = append(warnings, m.Name+" has multiple solution files; using "+manifest)
		}
	} else {
		projects, err = gen.Find(m.WorktreePath, max(4, o.Depth), "*.csproj", "*.fsproj", "*.vbproj")
		if err != nil {
			return nil, err
		}
		if len(projects) == 0 {
			return nil, nil
		}
		manifest = projects[0]
	}
	return &gen.Detection{
		Member: m, Manifest: manifest,
		Facts: map[string]any{"projects": projects, "warnings": warnings},
	}, nil
}

// solutionXML ignores solution settings and folders but keeps projects at every folder depth.
type solutionXML struct {
	XMLName  xml.Name
	Projects []projectXML  `xml:"Project"`
	Folders  []solutionXML `xml:"Folder"`
}

type projectXML struct {
	Path string `xml:"Path,attr"`
}

func (s solutionXML) paths() []string {
	var out []string
	for _, p := range s.Projects {
		out = append(out, p.Path)
	}
	for _, f := range s.Folders {
		out = append(out, f.paths()...)
	}
	return out
}

var slnProject = regexp.MustCompile(`^\s*Project\("[^"\r\n]+"\)\s*=\s*"[^"\r\n]*",\s*"([^"\r\n]+)",\s*"[^"\r\n]+"\s*$`)

func readSolution(root, manifest string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifest))) //nolint:gosec // G304: Find returns a file inside the worktree
	if err != nil {
		return nil, err
	}
	var paths []string
	if path.Ext(manifest) == ".slnx" {
		var solution solutionXML
		if err := xml.Unmarshal(b, &solution); err != nil {
			return nil, err
		}
		if solution.XMLName.Local != "Solution" {
			return nil, fmt.Errorf("expected Solution element, got %s", solution.XMLName.Local)
		}
		paths = solution.paths()
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(b))
		for scanner.Scan() {
			line := scanner.Text()
			if match := slnProject.FindStringSubmatch(line); match != nil {
				paths = append(paths, match[1])
			} else if strings.HasPrefix(strings.TrimSpace(line), "Project(") {
				return nil, fmt.Errorf("invalid solution project: %s", line)
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	var projects []string
	for _, p := range paths {
		p = strings.ReplaceAll(p, `\`, "/")
		if !isProject(p) {
			continue // Legacy solutions also list solution folders as projects.
		}
		if path.IsAbs(p) || strings.Contains(p, ":") {
			return nil, fmt.Errorf("project path %q is not relative to the worktree", p)
		}
		p = path.Join(path.Dir(manifest), p)
		if p == ".." || strings.HasPrefix(p, "../") {
			return nil, fmt.Errorf("project path %q leaves the worktree", p)
		}
		linked, err := hasSymlink(root, p)
		if err != nil {
			return nil, err
		}
		if !linked {
			projects = append(projects, p)
		}
	}
	slices.Sort(projects)
	return slices.Compact(projects), nil
}

func hasSymlink(root, p string) (bool, error) {
	for part := range strings.SplitSeq(p, "/") {
		root = filepath.Join(root, part)
		info, err := os.Lstat(root)
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

func isProject(p string) bool {
	return slices.Contains([]string{".csproj", ".fsproj", ".vbproj"}, path.Ext(p))
}

func (Generator) Plan(s gen.Space, found []gen.Detection) (gen.Plan, error) {
	cfg, err := gen.DecodeConfig(s.Config, Config{Format: FormatSLNX})
	if err != nil {
		return gen.Plan{}, err
	}
	if cfg.Format != FormatSLNX && cfg.Format != FormatSLN {
		return gen.Plan{}, fmt.Errorf("invalid dotnet format %q: want slnx or sln", cfg.Format)
	}
	found = slices.Clone(found)
	slices.SortFunc(found, func(a, b gen.Detection) int { return strings.Compare(a.Member.Name, b.Member.Name) })
	var plan gen.Plan
	var members []gen.Detection
	for _, d := range found {
		warnings, _ := d.Facts["warnings"].([]string)
		plan.Warnings = append(plan.Warnings, warnings...)
		projects, _ := d.Facts["projects"].([]string)
		if len(projects) > 0 {
			members = append(members, d)
		}
	}
	if len(members) == 0 {
		return plan, nil
	}
	var content string
	if cfg.Format == FormatSLNX {
		content = renderSLNX(s.Name, members)
	} else {
		content, err = renderSLN(s.Name, members)
		if err != nil {
			return gen.Plan{}, err
		}
	}
	plan.Files = []gen.File{{Path: s.Name + "." + string(cfg.Format), Content: []byte(content)}}
	return plan, nil
}

func projectPaths(d gen.Detection) []string {
	projects, _ := d.Facts["projects"].([]string)
	projects = slices.Clone(projects)
	slices.Sort(projects)
	return slices.Compact(projects)
}

func xmlText(s string) string {
	var b bytes.Buffer
	// A bytes.Buffer cannot fail a write.
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func renderSLNX(space string, members []gen.Detection) string {
	var b strings.Builder
	b.WriteString("<!-- " + gen.Header(space) + " -->\n<Solution>\n")
	for _, d := range members {
		b.WriteString("  <Folder Name=\"/" + xmlText(d.Member.Name) + "/\">\n")
		for _, p := range projectPaths(d) {
			b.WriteString("    <Project Path=\"./" + xmlText(path.Join(d.Member.Name, p)) + "\" />\n")
		}
		b.WriteString("  </Folder>\n")
	}
	b.WriteString("</Solution>\n")
	return b.String()
}

const folderType = "{2150E333-8FDC-42A3-9474-1A3956D46DE8}"

func projectType(p string) string {
	switch path.Ext(p) {
	case ".fsproj":
		return "{F2A71F9B-5D33-465A-A702-920D77279786}"
	case ".vbproj":
		return "{F184B08F-C81C-45F6-A57F-5ABD9991F28F}"
	default:
		return "{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}"
	}
}

// guid uses UUIDv5 with the DNS namespace and a space-relative path, so moves change identity
// but repeated generation and changes to another project do not.
func guid(p string) string {
	namespace := []byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	h := sha1.New() //nolint:gosec // G401: UUIDv5 requires SHA-1; this is an identifier, not a security hash
	_, _ = h.Write(namespace)
	_, _ = h.Write([]byte(p))
	id := h.Sum(nil)[:16]
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("{%08X-%04X-%04X-%04X-%012X}", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func renderSLN(space string, members []gen.Detection) (string, error) {
	var b strings.Builder
	b.WriteString("# " + gen.Header(space) + "\nMicrosoft Visual Studio Solution File, Format Version 12.00\n# Visual Studio Version 17\n")
	var projects, nesting []string
	for _, d := range members {
		if strings.ContainsAny(d.Member.Name, "\"\r\n") {
			return "", fmt.Errorf("member name %q cannot be written in sln format", d.Member.Name)
		}
		folderID := guid(d.Member.Name + "/")
		fmt.Fprintf(&b, "Project(\"%s\") = \"%s\", \"%s\", \"%s\"\nEndProject\n", folderType, d.Member.Name, d.Member.Name, folderID)
		for _, p := range projectPaths(d) {
			p = "./" + path.Join(d.Member.Name, p)
			if strings.ContainsAny(p, "\"\r\n") {
				return "", fmt.Errorf("project path %q cannot be written in sln format", p)
			}
			id := guid(p)
			name := strings.TrimSuffix(path.Base(p), path.Ext(p))
			fmt.Fprintf(&b, "Project(\"%s\") = \"%s\", \"%s\", \"%s\"\nEndProject\n", projectType(p), name, p, id)
			projects = append(projects, id)
			nesting = append(nesting, "\t\t"+id+" = "+folderID+"\n")
		}
	}
	b.WriteString("Global\n\tGlobalSection(SolutionConfigurationPlatforms) = preSolution\n\t\tDebug|Any CPU = Debug|Any CPU\n\t\tRelease|Any CPU = Release|Any CPU\n\tEndGlobalSection\n\tGlobalSection(ProjectConfigurationPlatforms) = postSolution\n")
	for _, id := range projects {
		for _, configuration := range []string{"Debug", "Release"} {
			fmt.Fprintf(&b, "\t\t%s.%s|Any CPU.ActiveCfg = %s|Any CPU\n\t\t%s.%s|Any CPU.Build.0 = %s|Any CPU\n", id, configuration, configuration, id, configuration, configuration)
		}
	}
	b.WriteString("\tEndGlobalSection\n\tGlobalSection(NestedProjects) = preSolution\n")
	for _, line := range nesting {
		b.WriteString(line)
	}
	b.WriteString("\tEndGlobalSection\nEndGlobal\n")
	return b.String(), nil
}
