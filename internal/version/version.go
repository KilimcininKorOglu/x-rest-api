// Package version holds the single source of truth for the application version.
package version

// Version is the current release version. It is bumped in place (no ldflags),
// so a build always reports the version committed to source.
const Version = "1.0.0"
