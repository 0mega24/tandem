// Package version holds the build version of tdm.
//
// The Makefile and Nix set Version, Commit and Date with -ldflags -X. A plain `go install` build
// leaves them empty, so String falls back to the embedded VERSION file and the VCS data that the
// Go toolchain records in the binary.
package version

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

// Set by -ldflags -X. Empty in a plain go build.
var (
	Version string
	Commit  string
	Date    string
)

//go:embed VERSION
var base string

// Info is the resolved version information of the running binary.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Get resolves the version information. The ldflags values win; the build info fills the gaps.
func Get() Info {
	info := Info{Version: Version, Commit: Commit, Date: Date}
	if info.Version == "" {
		info.Version = strings.TrimSpace(base)
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if info.Date == "" {
				info.Date = s.Value
			}
		}
	}
	return info
}

// String formats the information as one line, for `tdm version`.
func (i Info) String() string {
	var b strings.Builder
	b.WriteString("tdm ")
	b.WriteString(i.Version)
	if i.Commit != "" {
		b.WriteString(" (" + i.Commit)
		if i.Date != "" {
			b.WriteString(", " + i.Date)
		}
		b.WriteString(")")
	}
	return b.String()
}
