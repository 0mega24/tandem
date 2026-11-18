package gen

import (
	"slices"
	"strings"
	"testing"
)

type fakeGen struct{ meta Meta }

func (f fakeGen) Meta() Meta                          { return f.meta }
func (fakeGen) Detect(Member) (*Detection, error)     { return nil, nil }
func (fakeGen) Plan(Space, []Detection) (Plan, error) { return Plan{}, nil }

// withRegistry runs the test with only the given generators registered.
func withRegistry(t *testing.T, gens ...Generator) {
	t.Helper()
	mu.Lock()
	saved := registry
	registry = map[string]Generator{}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		registry = saved
		mu.Unlock()
	})
	for _, g := range gens {
		Register(g)
	}
}

func TestRegistry(t *testing.T) {
	withRegistry(t,
		fakeGen{Meta{ID: "rust", DefaultOn: true}},
		fakeGen{Meta{ID: "go", DefaultOn: true}},
		fakeGen{Meta{ID: "uv"}},
	)

	if got := IDs(); !slices.Equal(got, []string{"go", "rust", "uv"}) {
		t.Errorf("IDs() = %v, want sorted", got)
	}
	if _, ok := ByID("rust"); !ok {
		t.Error("ByID(rust) not found")
	}

	tests := []struct {
		enabled []string
		want    []string
	}{
		{nil, []string{"go", "rust"}},
		{[]string{"none"}, []string{}},
		{[]string{}, []string{}},
		{[]string{"uv", "go", "uv"}, []string{"go", "uv"}},
	}
	for _, tt := range tests {
		got, err := Select(tt.enabled)
		if err != nil || !slices.Equal(got, tt.want) {
			t.Errorf("Select(%v) = %v, %v; want %v", tt.enabled, got, err, tt.want)
		}
	}

	if _, err := Select([]string{"zig"}); err == nil || !strings.Contains(err.Error(), "known: go, rust, uv") {
		t.Errorf("Select(zig) err = %v, want the known IDs", err)
	}
}

func TestRegisterPanics(t *testing.T) {
	withRegistry(t, fakeGen{Meta{ID: "go"}})
	for _, id := range []string{"go", "", "a b"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", id)
				}
			}()
			Register(fakeGen{Meta{ID: id}})
		}()
	}
}

func TestDecodeConfig(t *testing.T) {
	type cfg struct {
		Mode       string   `toml:"mode"`
		Registries []string `toml:"registries"`
	}
	def := cfg{Mode: "auto"}

	got, err := DecodeConfig(map[string]any{"registries": []any{"my"}}, def)
	if err != nil || got.Mode != "auto" || !slices.Equal(got.Registries, []string{"my"}) {
		t.Errorf("DecodeConfig = %+v, %v; want the default mode and one registry", got, err)
	}
	if _, err := DecodeConfig(map[string]any{"modes": "patch"}, def); err == nil {
		t.Error("DecodeConfig with a typo = nil, want an error")
	}
	if got, err := DecodeConfig(nil, def); err != nil || got.Mode != "auto" {
		t.Errorf("DecodeConfig(nil) = %+v, %v; want the defaults", got, err)
	}
}
