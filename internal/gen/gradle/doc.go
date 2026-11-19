// Package gradle writes settings.gradle.kts in the space root. It includes the member
// builds with includeBuild so Gradle can substitute local projects for published dependencies.
// Run a member's wrapper with the space root as the project directory, for example
// ./api/gradlew -p . build.
//
//tandem:generator id=gradle manifest=settings.gradle.kts
package gradle
