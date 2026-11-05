package dbutil

// DropByOrg deletes every entry of m whose org (per orgOf) equals orgID. Services
// use it to replace exactly one org's cached rows after reloading them, instead of
// rebuilding the cache for every org.
func DropByOrg[T any](m map[string]T, orgID string, orgOf func(T) string) {
	for k, v := range m {
		if orgOf(v) == orgID {
			delete(m, k)
		}
	}
}
