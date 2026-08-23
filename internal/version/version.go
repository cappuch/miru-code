package version

import (
	"regexp"
	"strconv"
	"strings"
)

// Version matches ts_miru_code package.json.
const Version = "1.5.3"

var semverRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

// MiruVersion returns the package version string.
func MiruVersion() string {
	return Version
}

// IndexCacheEpoch mirrors version.ts indexCacheEpoch.
// 0.x → "0.{minor}"; 1.x+ → major only.
func IndexCacheEpoch() string {
	parts := strings.Split(Version, ".")
	major := "0"
	minor := "0"
	if len(parts) > 0 && parts[0] != "" {
		major = parts[0]
	}
	if len(parts) > 1 && parts[1] != "" {
		minor = parts[1]
	}
	if major == "0" {
		return "0." + minor
	}
	return major
}

// IsVersionNewer reports whether latest is newer than current (semver x.y.z prefix).
func IsVersionNewer(latest, current string) bool {
	a := parseSemver(latest)
	b := parseSemver(current)
	if a == nil || b == nil {
		return latest != current
	}
	for i := 0; i < 3; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) *[3]int {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return nil
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, _ := strconv.Atoi(m[i+1])
		out[i] = n
	}
	return &out
}
