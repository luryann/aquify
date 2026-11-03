package workers

import (
	"errors"
	"testing"
	"time"
)

func TestRetryDelayUsesQueuePolicy(t *testing.T) {
	err := &queueError{queue: QueueBilling, err: errors.New("boom")}
	if got := retryDelay(0, err, nil); got != time.Minute {
		t.Fatalf("billing first retry = %v", got)
	}
	if got := retryDelay(1, err, nil); got != 10*time.Minute {
		t.Fatalf("billing second retry = %v", got)
	}
	if got := retryDelay(9, err, nil); got != time.Hour {
		t.Fatalf("billing capped = %v", got)
	}
	if got := retryDelay(0, errors.New("plain"), nil); got <= 0 {
		t.Fatalf("default delay = %v", got)
	}
}
