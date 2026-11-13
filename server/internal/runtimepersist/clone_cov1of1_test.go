package runtimepersist

import (
	"reflect"
	"testing"
)

type cov1xRec struct {
	N    int
	Name string
}

func TestCov1xCloneMapIsIndependentAndNonNil(t *testing.T) {
	src := map[string]int{"a": 1}
	cp := CloneMap(src)
	cp["a"] = 2
	cp["b"] = 3
	if src["a"] != 1 || len(src) != 1 {
		t.Fatalf("source mutated: %v", src)
	}
	if fresh := CloneMap(src); !reflect.DeepEqual(fresh, src) {
		t.Fatalf("clone differs from source: %v vs %v", fresh, src)
	}
	nilCopy := CloneMap[string, int](nil)
	if nilCopy == nil {
		t.Fatal("clone of nil must be non-nil")
	}
	nilCopy["x"] = 1
}

func TestCov1xCloneSetMapDeepCopies(t *testing.T) {
	src := map[string]map[string]struct{}{"g": {"s1": {}}}
	cp := CloneSetMap(src)
	cp["g"]["s2"] = struct{}{}
	cp["h"] = nil
	if _, ok := cp["g"]["s1"]; !ok {
		t.Fatalf("copy lost original member: %v", cp)
	}
	if len(src["g"]) != 1 || len(src) != 1 {
		t.Fatalf("source mutated: %v", src)
	}
	if CloneSetMap(nil) == nil {
		t.Fatal("nil input must yield non-nil map")
	}
}

func TestCov1xClonePtrsCopiesValuesKeepsNil(t *testing.T) {
	a := &cov1xRec{N: 1, Name: "a"}
	cp := ClonePtrs([]*cov1xRec{a, nil})
	if len(cp) != 2 || cp[1] != nil {
		t.Fatalf("unexpected result %v", cp)
	}
	if cp[0] == a || !reflect.DeepEqual(cp[0], a) {
		t.Fatal("expected distinct equal copy")
	}
	cp[0].N = 9
	if a.N != 1 {
		t.Fatal("source mutated")
	}
	if got := ClonePtrs[cov1xRec](nil); got == nil || len(got) != 0 {
		t.Fatalf("nil input should give empty non-nil slice, got %#v", got)
	}
}

func TestCov1xClonePtrMapCopiesValuesKeepsNil(t *testing.T) {
	a := &cov1xRec{N: 1}
	cp := ClonePtrMap(map[string]*cov1xRec{"a": a, "n": nil})
	if cp["a"] == a || cp["a"].N != 1 {
		t.Fatal("expected distinct copy")
	}
	if v, ok := cp["n"]; !ok || v != nil {
		t.Fatal("nil entry should stay nil")
	}
	cp["a"].N = 5
	if a.N != 1 {
		t.Fatal("source mutated")
	}
}

func TestCov1xClonePtrListMapCopiesDeeply(t *testing.T) {
	a := &cov1xRec{N: 1}
	src := map[string][]*cov1xRec{"k": {a, nil}}
	cp := ClonePtrListMap(src)
	if cp["k"][0] == a || cp["k"][0].N != 1 || cp["k"][1] != nil {
		t.Fatalf("bad copy %v", cp["k"])
	}
	cp["k"][0].N = 7
	if a.N != 1 {
		t.Fatal("source mutated")
	}
}
