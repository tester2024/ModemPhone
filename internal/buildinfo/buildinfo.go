// Package buildinfo reports which build is running.
//
// The values are injected at link time by GoReleaser, so a released binary can
// be traced back to its commit. A plain `go build` leaves them empty and the
// app then reports itself as a development build.
package buildinfo

// Version and Commit are set with -ldflags "-X" at build time.
var (
	Version = ""
	Commit  = ""
)

// Short returns the commit hash shortened for display, or an empty string.
func Short() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}
	return Commit
}

// Describe is a one-line summary for the diagnostics block and window title.
func Describe() string {
	v := Version
	if v == "" {
		v = "dev"
	}
	if c := Short(); c != "" {
		return v + " (" + c + ")"
	}
	return v
}
