// Package version provides version information for the wallet application.
// Version values are typically injected at build time using ldflags.
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables (set via ldflags)
var (
	// Version is the semantic version (e.g., "1.0.0")
	Version = "1.1.0"

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

// Info contains complete version information
type Info struct {
	Version    string `json:"version"`
	GitCommit  string `json:"git_commit"`
	GitBranch  string `json:"git_branch"`
	BuildDate  string `json:"build_date"`
	GoVersion  string `json:"go_version"`
	Platform   string `json:"platform"`
	Prerelease string `json:"prerelease,omitempty"`
}

// GetInfo returns the complete version information
func GetInfo() Info {
	return Info{
		Version:    Version,
		GitCommit:  GitCommit,
		GitBranch:  GitBranch,
		BuildDate:  BuildDate,
		GoVersion:  runtime.Version(),
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Prerelease: Prerelease,
	}
}

// String returns the full version string
func String() string {
	if Prerelease != "" {
		return fmt.Sprintf("v%s-%s", Version, Prerelease)
	}
	return fmt.Sprintf("v%s", Version)
}

// Short returns just the version number with prerelease
func Short() string {
	if Prerelease != "" {
		return fmt.Sprintf("%s-%s", Version, Prerelease)
	}
	return Version
}

// Full returns the version with git commit
func Full() string {
	commit := GitCommit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if Prerelease != "" {
		return fmt.Sprintf("v%s-%s (%s)", Version, Prerelease, commit)
	}
	return fmt.Sprintf("v%s (%s)", Version, commit)
}
