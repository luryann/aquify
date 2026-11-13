package main

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// cov1xUseFakeRedis points REDIS_URL at an in-process Redis for the test's lifetime. The main and
// worker tests need a Redis that accepts connections; a private fake keeps them deterministic,
// runnable without any service, and free of keys left behind in a shared Redis.
func cov1xUseFakeRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	t.Setenv("REDIS_URL", "redis://"+mr.Addr())
	return mr
}
