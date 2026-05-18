package buildinfo

var (
	Version = "0.1.0-dev"
	Commit  = ""
)

func String() string {
	if Commit != "" {
		return Version + " (" + Commit + ")"
	}
	return Version
}
