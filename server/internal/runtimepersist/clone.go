package runtimepersist

import "maps"

// CloneMap copies in into a new, always non-nil map, so snapshot restores can
// keep writing to the result even when the snapshot field was absent.
func CloneMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	maps.Copy(out, in)
	return out
}

// CloneSetMap deep-copies a map of string sets.
func CloneSetMap(in map[string]map[string]struct{}) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(in))
	for key, values := range in {
		out[key] = CloneMap(values)
	}
	return out
}

func clonePtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

// ClonePtrs copies a slice of struct pointers (nil entries stay nil).
func ClonePtrs[T any](in []*T) []*T {
	out := make([]*T, 0, len(in))
	for _, v := range in {
		out = append(out, clonePtr(v))
	}
	return out
}

// ClonePtrMap copies a map of struct pointers (nil entries stay nil).
func ClonePtrMap[T any](in map[string]*T) map[string]*T {
	out := make(map[string]*T, len(in))
	for k, v := range in {
		out[k] = clonePtr(v)
	}
	return out
}

// ClonePtrListMap copies a map of struct-pointer slices.
func ClonePtrListMap[T any](in map[string][]*T) map[string][]*T {
	out := make(map[string][]*T, len(in))
	for k, values := range in {
		out[k] = ClonePtrs(values)
	}
	return out
}
