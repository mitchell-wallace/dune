package version

// Set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
)

func String() string {
	return Version + " (" + Commit + ")"
}
