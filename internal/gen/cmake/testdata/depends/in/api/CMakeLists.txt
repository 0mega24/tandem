project(api)
FetchContent_Declare(SharedAlias GIT_REPOSITORY https://github.com/org/shared.git)
CPMAddPackage(NAME cpmAlias GIT_REPOSITORY git@github.com:org/shared.git)
FetchContent_Declare(External GIT_REPOSITORY https://github.com/org/external)
