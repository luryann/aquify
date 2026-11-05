package dbutil

import "testing"

func TestForeignClock(t *testing.T) {
	var c ForeignClock
	if c.Current("a", 0) {
		t.Fatal("an org that was never marked is not current")
	}
	c.Mark("a", 3)
	if !c.Current("a", 3) {
		t.Fatal("marked at 3, probed 3: current")
	}
	if c.Current("a", 4) {
		t.Fatal("another process wrote (3 -> 4): not current")
	}
	if c.Current("b", 3) {
		t.Fatal("orgs are independent")
	}
	c.Forget("a")
	if c.Current("a", 3) {
		t.Fatal("forgotten: not current")
	}
}
