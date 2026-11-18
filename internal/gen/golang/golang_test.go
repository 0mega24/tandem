package golang

import (
	"testing"

	"github.com/0mega24/tandem/internal/gen/gentest"
)

func TestGolden(t *testing.T) {
	gentest.Run(t, Generator{})
}

func TestGoDirective(t *testing.T) {
	tests := []struct {
		in, want string
		err      bool
	}{
		{"module x\n\ngo 1.22.3 // pinned\n", "1.22.3", false},
		{"module x\n", "", false},
		{"module x\ngo banana\n", "", true},
		{"module x\n// go 1.30\ngo 1.21\n", "1.21", false},
	}
	for _, tt := range tests {
		got, err := goDirective([]byte(tt.in))
		if got != tt.want || (err != nil) != tt.err {
			t.Errorf("goDirective(%q) = %q, %v; want %q (error %v)", tt.in, got, err, tt.want, tt.err)
		}
	}
}
