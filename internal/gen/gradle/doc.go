// Package gradle writes settings.gradle.kts in the space root to connect Gradle builds through
// composite-build dependency substitution. Run a member wrapper from the space root, for example
// ./api/gradlew -p . build, to use the included builds.
//
//tandem:generator id=gradle manifest=settings.gradle.kts
package gradle
