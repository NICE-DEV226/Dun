// Package version holds build-time version information for Dun.
package version

var (
	// Version is the current semantic version.
	Version = "0.1.0"
	// Commit is the git commit the binary was built from.
	Commit = "dev"
	// Date is the build date.
	Date = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return "dun " + Version + " (" + Commit + ", " + Date + ")"
}
