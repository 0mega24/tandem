package ui

import "testing"

func TestDiff(t *testing.T) {
	tests := []struct {
		name, a, b, want string
	}{
		{"equal", "x\n", "x\n", ""},
		{"new file", "", "a\nb\n", "--- f\n+++ f (planned)\n@@ -0,0 +1,2 @@\n+a\n+b\n"},
		{"deleted", "a\n", "", "--- f\n+++ f (planned)\n@@ -1,1 +0,0 @@\n-a\n"},
		{
			"change in the middle",
			"1\n2\n3\n4\n5\n6\n7\n8\n9\n",
			"1\n2\n3\n4\nfive\n6\n7\n8\n9\n",
			"--- f\n+++ f (planned)\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n",
		},
		{
			"two hunks",
			"a\n1\n2\n3\n4\n5\n6\n7\n8\nb\n",
			"A\n1\n2\n3\n4\n5\n6\n7\n8\nB\n",
			"--- f\n+++ f (planned)\n@@ -1,4 +1,4 @@\n-a\n+A\n 1\n 2\n 3\n@@ -7,4 +7,4 @@\n 6\n 7\n 8\n-b\n+B\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Diff("f", []byte(tt.a), []byte(tt.b)); got != tt.want {
				t.Errorf("Diff:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
