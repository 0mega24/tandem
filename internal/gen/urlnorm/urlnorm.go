// Package urlnorm compares git remote URLs, so a generator can find the member whose origin is a
// dependency's git URL (docs/spec/generators.md, Rust rule 3).
package urlnorm

import "strings"

// Normalize returns the form "host/path" of a git URL: no scheme, no user, no ".git" and no
// trailing "/". git@host:org/repo, ssh://git@host/org/repo.git and https://host/org/repo/ give
// the same result. The host is lower case; the path keeps its case, as servers differ there.
func Normalize(url string) string {
	u := strings.TrimSpace(url)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	} else if at, colon := strings.Index(u, "@"), strings.Index(u, ":"); colon > 0 && (at < 0 || at < colon) {
		u = u[:colon] + "/" + u[colon+1:] // scp form: [user@]host:path
	}
	if i := strings.Index(u, "@"); i >= 0 && i < strings.Index(u+"/", "/") {
		u = u[i+1:] // user in the host part
	}

	u = strings.TrimSuffix(strings.TrimRight(u, "/"), ".git")
	u = strings.TrimRight(u, "/")
	host, path, _ := strings.Cut(u, "/")
	if path == "" {
		return strings.ToLower(host)
	}
	return strings.ToLower(host) + "/" + path
}

// Same reports whether a and b name the same repo. An empty URL is the same as nothing.
func Same(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return Normalize(a) == Normalize(b)
}
