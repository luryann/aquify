package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/luryann/aquify/server/internal/testutil"
)

func cov1xAppend(t *testing.T, s *Service, req AppendRequest) *AuditEvent {
	t.Helper()
	ev, err := s.Append(req)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	return ev
}

func TestCov1xAppendDefaultsAndSnapshots(t *testing.T) {
	t.Parallel()
	s := NewService()
	s.SetPool(nil) // nil pool must leave the service in-memory
	if s.repo != nil {
		t.Fatal("nil pool must not wire a repository")
	}
	ev := cov1xAppend(t, s, AppendRequest{
		OrgID: "o", Action: ActionAuthRoleAssign,
		Before: map[string]string{"role": "parent"}, After: map[string]string{"role": "coach"},
	})
	if ev.ID == "" || len(ev.ID) != 32 {
		t.Fatalf("id = %q", ev.ID)
	}
	if ev.ActorRoles == nil || len(ev.ActorRoles) != 0 || ev.Metadata == nil {
		t.Fatalf("nil roles/metadata should default to empty: %+v", ev)
	}
	if string(ev.BeforeState) != `{"role":"parent"}` || string(ev.AfterState) != `{"role":"coach"}` {
		t.Fatalf("snapshots = %s / %s", ev.BeforeState, ev.AfterState)
	}
}

func TestCov1xAppendRejectsUnmarshalableSnapshots(t *testing.T) {
	t.Parallel()
	s := NewService()
	if _, err := s.Append(AppendRequest{OrgID: "o", Before: make(chan int)}); err == nil {
		t.Fatal("expected error for unmarshalable Before")
	}
	if _, err := s.Append(AppendRequest{OrgID: "o", After: func() {}}); err == nil {
		t.Fatal("expected error for unmarshalable After")
	}
	got, _ := s.Query(Filter{OrgID: "o"})
	if len(got) != 0 {
		t.Fatalf("failed appends must not be recorded: %+v", got)
	}
}

func TestCov1xQueryInMemoryFilters(t *testing.T) {
	t.Parallel()
	s := NewService()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.events = []AuditEvent{
		{ID: "1", OrgID: "o", ActorUserID: "u1", Action: "A.x", ResourceType: "Swimmer", ResourceID: "r1", OccurredAt: base},
		{ID: "2", OrgID: "o", ActorUserID: "u2", Action: "a.x", ResourceType: "swimmer", ResourceID: "r1", OccurredAt: base.Add(time.Hour)},
		{ID: "3", OrgID: "o", ActorUserID: "u1", Action: "b.y", ResourceType: "meet", ResourceID: "r2", OccurredAt: base.Add(2 * time.Hour)},
		{ID: "4", OrgID: "other", ActorUserID: "u1", Action: "a.x", ResourceType: "swimmer", ResourceID: "r1", OccurredAt: base.Add(3 * time.Hour)},
	}
	ids := func(f Filter) string {
		t.Helper()
		got, err := s.Query(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range got {
			out = append(out, e.ID)
		}
		return strings.Join(out, ",")
	}
	from, to := base.Add(30*time.Minute), base.Add(90*time.Minute)
	cases := []struct {
		name string
		f    Filter
		want string
	}{
		{"org", Filter{OrgID: "o"}, "3,2,1"},
		{"all orgs", Filter{}, "4,3,2,1"},
		{"actor", Filter{OrgID: "o", ActorUserID: "u1"}, "3,1"},
		{"action case-insensitive", Filter{OrgID: "o", Action: "A.X"}, "2,1"},
		{"resource type case-insensitive", Filter{OrgID: "o", ResourceType: "SWIMMER"}, "2,1"},
		{"resource id", Filter{OrgID: "o", ResourceID: "r2"}, "3"},
		{"from", Filter{OrgID: "o", From: &from}, "3,2"},
		{"to", Filter{OrgID: "o", To: &to}, "2,1"},
		{"from+to", Filter{OrgID: "o", From: &from, To: &to}, "2"},
		{"limit", Filter{OrgID: "o", Limit: 2}, "3,2"},
		{"cursor", Filter{OrgID: "o", Cursor: "3"}, "2,1"},
		{"unknown cursor", Filter{OrgID: "o", Cursor: "zzz"}, ""},
		{"no match is empty non-nil", Filter{OrgID: "none"}, ""},
	}
	for _, c := range cases {
		if got := ids(c.f); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	empty, _ := s.Query(Filter{OrgID: "none"})
	if empty == nil {
		t.Fatal("expected non-nil empty slice")
	}
}

func TestCov1xDiff(t *testing.T) {
	t.Parallel()
	type rec struct {
		Name string   `json:"name"`
		Age  int      `json:"age"`
		Tags []string `json:"tags"`
	}
	d, err := Diff(
		map[string]any{"name": "a", "age": 1, "gone": true, "tags": []string{"x"}},
		map[string]any{"name": "b", "age": 1, "new": 2, "tags": []string{"x"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != 3 {
		t.Fatalf("diff = %+v", d)
	}
	if string(d["name"].Before) != `"a"` || string(d["name"].After) != `"b"` {
		t.Fatalf("name = %+v", d["name"])
	}
	if string(d["gone"].Before) != "true" || d["gone"].After != nil {
		t.Fatalf("gone = %+v", d["gone"])
	}
	if d["new"].Before != nil || string(d["new"].After) != "2" {
		t.Fatalf("new = %+v", d["new"])
	}
	same, err := Diff(rec{"a", 1, []string{"x"}}, rec{"a", 1, []string{"x"}})
	if err != nil || len(same) != 0 {
		t.Fatalf("identical structs: %v %+v", err, same)
	}
	nested, _ := Diff(rec{"a", 1, []string{"x"}}, rec{"a", 1, []string{"x", "y"}})
	if len(nested) != 1 || string(nested["tags"].After) != `["x","y"]` {
		t.Fatalf("nested = %+v", nested)
	}
	if _, err := Diff(make(chan int), map[string]any{}); err == nil {
		t.Fatal("expected marshal error for before")
	}
	if _, err := Diff(map[string]any{}, make(chan int)); err == nil {
		t.Fatal("expected marshal error for after")
	}
	if _, err := Diff([]int{1}, map[string]any{}); err == nil {
		t.Fatal("expected unmarshal error: before is not an object")
	}
	if _, err := Diff(map[string]any{}, "str"); err == nil {
		t.Fatal("expected unmarshal error: after is not an object")
	}
}

func TestCov1xDBAppendQueryRoundTrip(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	s := NewService()
	s.SetPool(pool)
	org, other := "c0510000-0000-4000-8000-00000000000a", "c0510000-0000-4000-8000-00000000000b"
	e1 := cov1xAppend(t, s, AppendRequest{
		OrgID: org, ActorUserID: "u1", ActorRoles: []string{"admin"}, Action: "x.y.z",
		ResourceType: "Swimmer", ResourceID: "r1",
		Before: map[string]int{"a": 1}, After: map[string]int{"a": 2},
		Metadata: map[string]string{"reason": "test"},
	})
	e2 := cov1xAppend(t, s, AppendRequest{OrgID: org, ActorUserID: "u2", Action: "other.act", ResourceType: "meet", ResourceID: "r2"})
	cov1xAppend(t, s, AppendRequest{OrgID: other, Action: "x.y.z"})

	if len(s.events) != 0 {
		t.Fatal("DB-backed service must not keep memory events")
	}
	got, err := s.Query(Filter{OrgID: org})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	// Newest first; timestamp ties fall back to id DESC, as in Postgres.
	newest, oldest := e2, e1
	if e1.OccurredAt.After(e2.OccurredAt) || (e1.OccurredAt.Equal(e2.OccurredAt) && e1.ID > e2.ID) {
		newest, oldest = e1, e2
	}
	if got[0].ID != newest.ID || got[1].ID != oldest.ID {
		t.Fatalf("order = %+v", got)
	}
	if got[0].ID == e1.ID {
		got[0], got[1] = got[1], got[0] // normalise so got[0]=e2, got[1]=e1 below
	}
	g := got[1]
	if g.Metadata["reason"] != "test" || len(g.ActorRoles) != 1 || g.ActorRoles[0] != "admin" ||
		string(g.BeforeState) != `{"a": 1}` && string(g.BeforeState) != `{"a":1}` {
		t.Fatalf("round trip lost data: %+v (before=%s)", g, g.BeforeState)
	}
	if got[0].BeforeState != nil || got[0].AfterState != nil || got[0].Metadata == nil {
		t.Fatalf("null snapshots/metadata should read back nil/empty: %+v", got[0])
	}

	from, to := e1.OccurredAt.Add(-time.Hour), e2.OccurredAt.Add(time.Hour)
	filtered, err := s.Query(Filter{OrgID: org, ActorUserID: "u1", Action: "X.Y.Z", ResourceType: "swimmer",
		ResourceID: "r1", From: &from, To: &to, Limit: 5})
	if err != nil || len(filtered) != 1 || filtered[0].ID != e1.ID {
		t.Fatalf("filtered: %v %+v", err, filtered)
	}
	paged, err := s.Query(Filter{OrgID: org, Cursor: newest.ID, Limit: 1})
	if err != nil || len(paged) != 1 || paged[0].ID != oldest.ID {
		t.Fatalf("cursor: %v %+v", err, paged)
	}
	none, err := s.Query(Filter{OrgID: "c0510000-0000-4000-8000-00000000000c"})
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("empty: %v %#v", err, none)
	}
}

func TestCov1xDBInsertIsIdempotentAndRejectsBadMetadataRead(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	s := NewService()
	s.SetPool(pool)
	ev := cov1xAppend(t, s, AppendRequest{OrgID: "c0510000-0000-4000-8000-0000000000d0", Action: "a"})
	// Same id twice: ON CONFLICT DO NOTHING keeps one row.
	if err := s.repo.Insert(context.Background(), *ev); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Query(Filter{OrgID: "c0510000-0000-4000-8000-0000000000d0"})
	if len(got) != 1 {
		t.Fatalf("duplicate insert created rows: %+v", got)
	}
	// A corrupt (non-object) metadata value must surface as a decode error.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO audit_events (id, org_id, actor_user_id, actor_roles, action, resource_type, resource_id, metadata, occurred_at)
		 VALUES ('bad', 'c0510000-0000-4000-8000-0000000000d0', '', '{}', 'a', '', '', '"scalar"'::jsonb, now())`); err != nil {
		t.Fatal(err)
	}
	_, err := s.Query(Filter{OrgID: "c0510000-0000-4000-8000-0000000000d0"})
	if err == nil || !strings.Contains(err.Error(), "decode metadata") {
		t.Fatalf("err = %v", err)
	}
}

func TestCov1xDBFailureSurfaces(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	s := NewService()
	s.SetPool(pool)
	pool.Close()
	if _, err := s.Append(AppendRequest{OrgID: "c0510000-0000-4000-8000-0000000000d1", Action: "a"}); err == nil || !strings.Contains(err.Error(), "audit: insert event") {
		t.Fatalf("append err = %v", err)
	}
	if _, err := s.Query(Filter{OrgID: "c0510000-0000-4000-8000-0000000000d1"}); err == nil || !strings.Contains(err.Error(), "audit: query events") {
		t.Fatalf("query err = %v", err)
	}
}

func TestCov1xNullableRawMessage(t *testing.T) {
	t.Parallel()
	if nullableRawMessage(nil) != nil || nullableRawMessage(json.RawMessage{}) != nil {
		t.Fatal("empty must be nil")
	}
	b, ok := nullableRawMessage(json.RawMessage(`{"a":1}`)).([]byte)
	if !ok || string(b) != `{"a":1}` {
		t.Fatalf("got %v", b)
	}
}

func TestCov1xDBNullMetadataReadsAsEmptyMap(t *testing.T) {
	pool := testutil.IsolatedSchemaPool(t)
	s := NewService()
	s.SetPool(pool)
	org := "c0510000-0000-4000-8000-0000000000e0"
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO audit_events (id, org_id, actor_user_id, actor_roles, action, resource_type, resource_id, occurred_at)
		 VALUES ('nullmeta', $1, 'u', '{}', 'a', 't', 'r', now())`, org); err != nil {
		t.Fatal(err)
	}
	got, err := s.Query(Filter{OrgID: org})
	if err != nil || len(got) != 1 || got[0].Metadata == nil || len(got[0].Metadata) != 0 {
		t.Fatalf("got %v %+v", err, got)
	}
}
