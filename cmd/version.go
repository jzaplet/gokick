package main

// release is the build version, injected at link time via
//
//	-ldflags "-X main.release=<version>"
//
// The Makefile passes `git describe --tags`; the production Dockerfile and the
// release workflow pass the git tag. It becomes the Sentry release so issues
// group by deployed version. Empty in un-stamped builds.
var release string

// releaseVersion resolves the build version: the linker-injected value wins,
// falling back to envRelease (APP_SENTRY_RELEASE, read via config.LoadStartup)
// so a deploy can still set it via env when the binary wasn't stamped. Empty
// when neither is set (Sentry leaves the release unset, which it handles fine).
func releaseVersion(envRelease string) string {
	if release != "" {
		return release
	}
	return envRelease
}
