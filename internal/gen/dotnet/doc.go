// Package dotnet writes a solution file that groups member projects for editors and
// dotnet build. The format is .slnx by default, or .sln with the format option. The solution keeps
// package references; it does not replace them with references to local projects.
//
//tandem:generator id=dotnet manifest=*.slnx
package dotnet
