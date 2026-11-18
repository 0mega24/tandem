package vscode

import (
	"testing"

	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) { gentest.Run(t, Generator{}) }

func TestPlanRejectsAFolderOutsideTheRoot(t *testing.T) {
	m := gen.Member{Name: "api", WorktreePath: "/elsewhere/api"}
	if _, err := (Generator{}).Plan(gen.Space{Name: "feat", Root: "/space"}, []gen.Detection{{Member: m}}); err == nil {
		t.Fatal("Plan accepted a member outside the space root")
	}
}
