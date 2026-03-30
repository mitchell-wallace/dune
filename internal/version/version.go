package version

// Set via -ldflags at build time.
var (
	Version          = "dev"
	Commit           = "unknown"
	BaseImageRepo    = "ghcr.io/mitchell-wallace/dune-base"
	BaseImageVersion = "0.1.1"
)

func String() string {
	return Version + " (" + Commit + ")"
}

func BaseImageRef() string {
	return BaseImageRepo + ":" + BaseImageVersion
}
