package version

import "fmt"

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func Current() Info {
	return Info{
		Version: buildVersion,
		Commit:  buildCommit,
		Date:    buildDate,
	}
}

func (i Info) String() string {
	return fmt.Sprintf("%s (%s, %s)", i.Version, i.Commit, i.Date)
}
