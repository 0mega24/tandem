// Package cabal writes cabal.project in the space root so Cabal can build the local
// Haskell packages together. It reads package lists from member cabal.project files and finds
// .cabal files for members without a project file. Member project files stay unchanged.
//
//tandem:generator id=cabal manifest=cabal.project
package cabal
