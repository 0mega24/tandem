// Package ts writes a root tsconfig.json with references to member projects whose
// compilerOptions.composite is true. Run tsc --build from the space root to build the referenced
// projects. Member configs are read as JSON. This generator is off by default.
//
//tandem:generator id=ts manifest=tsconfig.json
package ts
