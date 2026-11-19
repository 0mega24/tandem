// Package uv writes a virtual uv workspace in the root pyproject.toml. It lists local
// Python packages and sets workspace sources for their dependencies on each other. Run uv from
// the space root to use one lock file and environment. This generator is off by default.
//
//tandem:generator id=uv manifest=pyproject.toml
package uv
