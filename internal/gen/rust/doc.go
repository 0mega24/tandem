// Package rust connects local Cargo crates without changing member manifests. In auto mode,
// it writes a root Cargo.toml workspace if no member contains a workspace. Otherwise it writes
// patch tables in .cargo/config.toml. An existing member workspace always requires patch mode,
// even when workspace mode was requested.
//
//tandem:generator id=rust manifest=Cargo.toml
package rust
