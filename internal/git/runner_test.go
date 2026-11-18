package git

import (
	"context"
	"errors"
	"testing"

	"github.com/0mega24/tandem/internal/git/testhelper"
)

func TestOriginURL(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()

	got, err := System{}.OriginURL(ctx, r.Path)
	if err != nil || got != r.Origin {
		t.Errorf("OriginURL() = %q, %v; want %q", got, err, r.Origin)
	}
	testhelper.Git(t, r.Path, "remote", "remove", "origin")
	got, err = System{}.OriginURL(ctx, r.Path)
	if err != nil || got != "" {
		t.Errorf("OriginURL() without origin = %q, %v; want empty and no error", got, err)
	}
}

func TestDefaultBranch(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()

	testhelper.Git(t, r.Path, "switch", "-q", "-c", "other")
	got, err := System{}.DefaultBranch(ctx, r.Path)
	if err != nil || got != "main" {
		t.Errorf("DefaultBranch() = %q, %v; want main from origin/HEAD", got, err)
	}
	testhelper.Git(t, r.Path, "remote", "set-head", "origin", "--delete")
	got, err = System{}.DefaultBranch(ctx, r.Path)
	if err != nil || got != "other" {
		t.Errorf("DefaultBranch() without origin/HEAD = %q, %v; want the checked-out branch", got, err)
	}
}

func TestBranchExistsAndDelete(t *testing.T) {
	testhelper.Isolate(t)
	dir := t.TempDir()
	r := testhelper.NewRepo(t, dir, "api")
	ctx := context.Background()
	s := System{}

	if ok, err := s.BranchExists(ctx, r.Path, "feat"); err != nil || ok {
		t.Fatalf("BranchExists(feat) before = %v, %v; want false", ok, err)
	}
	testhelper.Git(t, r.Path, "switch", "-q", "-c", "feat")
	testhelper.Commit(t, r.Path, "a.txt", "a\n", "unmerged")
	testhelper.Git(t, r.Path, "switch", "-q", "main")
	if ok, err := s.BranchExists(ctx, r.Path, "feat"); err != nil || !ok {
		t.Fatalf("BranchExists(feat) = %v, %v; want true", ok, err)
	}

	err := s.DeleteBranch(ctx, r.Path, "feat", false)
	if _, ok := errors.AsType[*Error](err); !ok {
		t.Fatalf("DeleteBranch(unmerged, no force) = %v, want a *git.Error", err)
	}
	if err := s.DeleteBranch(ctx, r.Path, "feat", true); err != nil {
		t.Fatalf("DeleteBranch(force) = %v", err)
	}
	if ok, _ := s.BranchExists(ctx, r.Path, "feat"); ok {
		t.Error("branch feat still exists after a forced delete")
	}
}
