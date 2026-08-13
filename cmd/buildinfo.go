// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import "log/slog"

// Build metadata, injected at link time:
//
//	go build -ldflags "-X main.Version=1.6.3 -X main.BuildDate=... -X main.VCSRef=..."
//
// These have to be package-level string variables with constant initializers: the
// linker silently ignores -X for a symbol it cannot find, which is exactly how every
// released image ended up reporting nothing at all.
//
// The build variant is deliberately not injectable. It is decided by the build tag
// (buildType, in init_cloud.go / init_selfhosted.go), so it can never disagree with
// the code that was actually compiled.
var (
	// Version is the release version, e.g. "1.6.3". "dev" for unversioned builds.
	Version = "dev"

	// BuildDate is the RFC 3339 UTC timestamp of the build.
	BuildDate = "unknown"

	// VCSRef is the git commit the binary was built from.
	VCSRef = "unknown"
)

// logBuildInfo records what this binary actually is, once, at startup. It is the
// only place the four values are reported, so a bug report can be tied to a commit.
func logBuildInfo(log *slog.Logger) {
	log.Info("WhenTo build info",
		"version", Version,
		"build_type", buildType,
		"build_date", BuildDate,
		"vcs_ref", VCSRef,
	)
}
