package ranking

import "os"

// SearchImprovementsEnabled matches features.ts (default true).
func SearchImprovementsEnabled() bool {
	v := os.Getenv("MIRU_SEARCH_V2")
	if v == "0" || v == "false" {
		return false
	}
	if v == "1" || v == "true" {
		return true
	}
	return true
}
