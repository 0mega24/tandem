// Package cmake writes .tandem/cmake/overrides.cmake. Load it with CMake's -C option
// when configuring a member build. It sets source folders for FetchContent and CPM dependencies
// whose git URLs match another member's origin. It reads literal dependency declarations and
// does not evaluate CMake variables.
//
//tandem:generator id=cmake manifest=CMakeLists.txt
package cmake
