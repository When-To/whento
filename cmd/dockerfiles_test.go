// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Three Dockerfiles build this package: the root one for `make docker-*`, and
// build/selfhosted plus build/cloud for CI and the releases. They cannot share
// anything — Docker has no include mechanism, and the two build/ paths are
// pinned by `file:` in the workflows, which pass no build argument that would
// let one file serve both variants. So the base image digests, the
// golang-migrate version and its two SHA-256 checksums are genuinely written out
// three times over.
//
// Duplication that cannot be removed can at least be made loud. This reads the
// files and fails when the copies disagree, which is the failure the triplication
// actually risks: a Dependabot bump landing in two files, or a checksum rotated
// in one, leaving a release image built from an image nobody reviewed.
//
// It lives here because cmd is the package these files build, and because a test
// is the only part of this repository that runs on every push without a Docker
// daemon.

// dockerfiles are the three copies that must agree.
var dockerfiles = []string{
	"Dockerfile",
	"build/selfhosted/Dockerfile",
	"build/cloud/Dockerfile",
}

var (
	// FROM [--platform=…] image[:tag][@sha256:…] [AS stage]
	fromPattern = regexp.MustCompile(`(?m)^FROM\s+(?:--platform=\S+\s+)?(\S+)`)
	// ARG NAME=value
	argPattern = regexp.MustCompile(`(?m)^ARG\s+([A-Z0-9_]+)=(\S+)`)
	// go install …/cmd/<tool>@<version>
	goInstallPattern = regexp.MustCompile(`(github\.com/[\w./-]+)@(v[\w.-]+)`)
	// MIGRATE_VERSION: vX.Y.Z, in the CI workflow's env block
	ciMigrateVersion = regexp.MustCompile(`(?m)^\s*MIGRATE_VERSION:\s*(\S+)`)
)

// pinnedArgs are the build arguments whose value is a supply-chain decision, and
// which therefore have to be the same in all three files.
var pinnedArgs = []string{
	"MIGRATE_VERSION",
	"MIGRATE_SHA256_AMD64",
	"MIGRATE_SHA256_ARM64",
}

func TestDockerfilesAgreeOnTheirPins(t *testing.T) {
	root := repoRoot(t)
	sources := readDockerfiles(t, root)

	t.Run("every base image is pinned by digest", func(t *testing.T) {
		for _, name := range dockerfiles {
			for _, ref := range fromPattern.FindAllStringSubmatch(sources[name], -1) {
				image := ref[1]
				// A FROM naming an earlier stage has no registry reference to pin.
				if !strings.ContainsAny(image, ":@") {
					continue
				}
				if !strings.Contains(image, "@sha256:") {
					t.Errorf("%s: FROM %s is not pinned by digest", name, image)
				}
			}
		}
	})

	t.Run("the same image tag resolves to the same digest everywhere", func(t *testing.T) {
		// tag -> file that first declared it, and the digest it declared.
		type pin struct{ file, digest string }
		seen := map[string]pin{}

		for _, name := range dockerfiles {
			for _, ref := range fromPattern.FindAllStringSubmatch(sources[name], -1) {
				tag, digest, ok := strings.Cut(ref[1], "@")
				if !ok {
					continue
				}
				first, known := seen[tag]
				if !known {
					seen[tag] = pin{file: name, digest: digest}

					continue
				}
				if first.digest != digest {
					t.Errorf("%s pins %s to %s, but %s pins it to %s",
						name, tag, digest, first.file, first.digest)
				}
			}
		}
	})

	t.Run("the golang-migrate pins are identical", func(t *testing.T) {
		for _, arg := range pinnedArgs {
			var first, firstFile string
			for _, name := range dockerfiles {
				value, ok := argValue(sources[name], arg)
				if !ok {
					t.Errorf("%s: %s is not declared", name, arg)

					continue
				}
				if firstFile == "" {
					first, firstFile = value, name

					continue
				}
				if value != first {
					t.Errorf("%s sets %s=%s, but %s sets %s=%s", name, arg, value, firstFile, arg, first)
				}
			}
		}
	})

	// The comment above those ARGs says to keep them in step with the CI
	// workflow and the devcontainer, because the CLI that validates the
	// migrations has to be the one that applies them. That is checkable.
	t.Run("golang-migrate matches CI and the devcontainer", func(t *testing.T) {
		want, ok := argValue(sources[dockerfiles[0]], "MIGRATE_VERSION")
		if !ok {
			t.Fatalf("%s declares no MIGRATE_VERSION", dockerfiles[0])
		}

		ci := readIfPresent(t, root, ".github/workflows/ci.yml")
		if match := ciMigrateVersion.FindStringSubmatch(ci); match != nil && match[1] != want {
			t.Errorf(".github/workflows/ci.yml pins golang-migrate to %s, the Dockerfiles to %s", match[1], want)
		}

		devcontainer := readIfPresent(t, root, ".devcontainer/Dockerfile")
		if got, ok := goInstallVersion(devcontainer, "golang-migrate/migrate"); ok && got != want {
			t.Errorf(".devcontainer/Dockerfile installs golang-migrate %s, the Dockerfiles pin %s", got, want)
		}
	})

	// The root Dockerfile is the only one that generates the Swagger spec, and
	// its own comment says the swag version must match the devcontainer's, so
	// that the spec an image ships is the one the annotations were written
	// against.
	t.Run("swag matches the devcontainer and CI", func(t *testing.T) {
		want, ok := goInstallVersion(sources[dockerfiles[0]], "swaggo/swag")
		if !ok {
			t.Skip("the root Dockerfile no longer installs swag")
		}

		for _, path := range []string{".devcontainer/Dockerfile", ".github/workflows/ci.yml"} {
			if got, ok := goInstallVersion(readIfPresent(t, root, path), "swaggo/swag"); ok && got != want {
				t.Errorf("%s installs swag %s, the root Dockerfile pins %s", path, got, want)
			}
		}
	})

	t.Run("every Dockerfile carries the BSL-1.1 header", func(t *testing.T) {
		for _, name := range dockerfiles {
			if !strings.Contains(sources[name], "# SPDX-License-Identifier: BSL-1.1") {
				t.Errorf("%s has no BSL-1.1 SPDX header", name)
			}
		}
	})
}

// argValue returns the value of `ARG NAME=value` in a Dockerfile.
func argValue(source, name string) (string, bool) {
	for _, match := range argPattern.FindAllStringSubmatch(source, -1) {
		if match[1] == name {
			return match[2], true
		}
	}

	return "", false
}

// goInstallVersion returns the version a `go install module@version` line pins,
// for the first module whose path contains substring.
func goInstallVersion(source, substring string) (string, bool) {
	for _, match := range goInstallPattern.FindAllStringSubmatch(source, -1) {
		if strings.Contains(match[1], substring) {
			return match[2], true
		}
	}

	return "", false
}

func readDockerfiles(t *testing.T, root string) map[string]string {
	t.Helper()

	sources := make(map[string]string, len(dockerfiles))
	for _, name := range dockerfiles {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sources[name] = string(content)
	}

	return sources
}

// readIfPresent returns the file's contents, or "" when it is not there. The
// files it reads are outside the Go build, so a checkout that does not carry
// them is not a test failure.
func readIfPresent(t *testing.T, root, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return ""
	}

	return string(content)
}

// repoRoot walks up from the test's working directory to the tree that holds
// go.work. Tests run in their own package directory, so the depth is not fixed.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Skip("the repository root was not found; nothing to check")

	return ""
}
