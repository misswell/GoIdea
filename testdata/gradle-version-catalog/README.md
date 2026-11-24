# Gradle published version catalog

`publisher` creates a real Maven publication from Gradle's `version-catalog`
plugin. `consumer` imports that publication in `settings.gradle` and checks
Gradle's generated library, `version.ref` and bundle accessors.

The acceptance test runs Gradle 8.10.2 with `--offline --no-daemon`, an
isolated `GRADLE_USER_HOME`, and a temporary Maven repository. Set
`GOIDEA_GRADLE_HOME` to the matching Gradle installation. No fixture writes to
the owner's Gradle cache.
