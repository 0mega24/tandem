# FetchContent_Declare(Comment GIT_REPOSITORY https://host/shared)
set(text "CPMAddPackage(NAME Text GIT_REPOSITORY https://host/shared)")
find_package(Shared REQUIRED)
FetchContent_Declare(Dynamic GIT_REPOSITORY ${shared_url})
