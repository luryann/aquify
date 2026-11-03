package workers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTaskInfoJSONOmitsZeroTimes(t *testing.T) {
	b, err := json.Marshal(TaskInfo{ID: "t1", Status: StatusDeadLetter})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "0001-01-01") || strings.Contains(string(b), "enqueued_at") || strings.Contains(string(b), "process_at") {
		t.Fatalf("zero times leaked: %s", b)
	}
	b, _ = json.Marshal(TaskInfo{ID: "t2", ProcessAt: time.Now()})
	if !strings.Contains(string(b), `"process_at"`) {
		t.Fatalf("process_at missing: %s", b)
	}
}
