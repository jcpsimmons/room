package codex

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const minimumSupportedVersion = "0.116.0"

var versionPattern = regexp.MustCompile(`\b(\d+)\.(\d+)\.(\d+)\b`)
var minimumVersion = Version{Major: 0, Minor: 116, Patch: 0}

type Version struct {
	Major int
	Minor int
	Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func ParseVersion(text string) (Version, error) {
	match := versionPattern.FindStringSubmatch(text)
	if len(match) != 4 {
		return Version{}, fmt.Errorf("could not parse Codex version from %q", strings.TrimSpace(text))
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return Version{}, err
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return Version{}, err
	}
	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return Version{}, err
	}
	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

func MinimumSupportedVersion() Version {
	return minimumVersion
}

func ValidateVersion(text string) error {
	actual, err := ParseVersion(text)
	if err != nil {
		return fmt.Errorf("%w; ROOM requires Codex %s or newer", err, minimumSupportedVersion)
	}
	if compareVersion(actual, MinimumSupportedVersion()) < 0 {
		return fmt.Errorf("unsupported Codex version %s; ROOM requires %s or newer", actual.String(), minimumSupportedVersion)
	}
	return nil
}

func compareVersion(a, b Version) int {
	switch {
	case a.Major != b.Major:
		return compareInt(a.Major, b.Major)
	case a.Minor != b.Minor:
		return compareInt(a.Minor, b.Minor)
	default:
		return compareInt(a.Patch, b.Patch)
	}
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
