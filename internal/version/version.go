// Package version provides version information for the wallet application.
// Version values are typically injected at build time using ldflags.
package version

import (
	"fmt"
)

// Build-time variables (set via ldflags)
var (
	// Version is the semantic version (e.g., "1.0.0")
	Version = "1.4.0"

	// GitCommit is the git commit hash
	GitCommit = "unknown"

	// GitBranch is the git branch name
	GitBranch = "unknown"

	// BuildDate is the build timestamp
	BuildDate = "unknown"

	// Prerelease is the prerelease tag (e.g., "alpha", "beta", "rc1")
	// Empty string for stable releases
	Prerelease = ""
)

// String returns the full version string
func String() string {
	if Prerelease != "" {
		return fmt.Sprintf("v%s-%s", Version, Prerelease)
	}
	return fmt.Sprintf("v%s", Version)
}
