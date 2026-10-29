package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisMetricsHistoryLimit = 500

// RedisJobMetrics stores queue execution counters and recent executions in
// Redis so they are visible across API and worker processes.
type RedisJobMetrics struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisJobMetrics(client redis.UniversalClient, prefix string) *RedisJobMetrics {
	if prefix == "" {
		prefix = "aquify:jobs:metrics"
	}
	return &RedisJobMetrics{client: client, prefix: prefix}
}

func (m *RedisJobMetrics) typeSetKey() string { return m.prefix + ":types" }
func (m *RedisJobMetrics) historyKey() string { return m.prefix + ":history" }
func (m *RedisJobMetrics) countersKey(taskType string) string {
	return fmt.Sprintf("%s:type:%s", m.prefix, taskType)
}

func (m *RedisJobMetrics) RecordEnqueue(taskType string) {
	m.incrementCounter(context.Background(), taskType, "enqueued")
}

func (m *RedisJobMetrics) RecordSuccess(exec JobExecution) {
	ctx := context.Background()
	m.incrementCounter(ctx, exec.Type, "succeeded")
	m.appendHistory(ctx, exec)
}

func (m *RedisJobMetrics) RecordFailure(exec JobExecution) {
	ctx := context.Background()
	m.incrementCounter(ctx, exec.Type, "failed")
	m.appendHistory(ctx, exec)
}

func (m *RedisJobMetrics) RecordDLQ(taskType string) {
	m.incrementCounter(context.Background(), taskType, "dlq")
}

func (m *RedisJobMetrics) incrementCounter(ctx context.Context, taskType, field string) {
	if taskType == "" || m == nil || m.client == nil {
		return
	}
	pipe := m.client.TxPipeline()
	pipe.SAdd(ctx, m.typeSetKey(), taskType)
	pipe.HIncrBy(ctx, m.countersKey(taskType), field, 1)
	_, _ = pipe.Exec(ctx)
}

func (m *RedisJobMetrics) appendHistory(ctx context.Context, exec JobExecution) {
	if m == nil || m.client == nil {
		return
	}
	raw, err := json.Marshal(exec)
	if err != nil {
		return
	}
	pipe := m.client.TxPipeline()
	pipe.LPush(ctx, m.historyKey(), raw)
	pipe.LTrim(ctx, m.historyKey(), 0, redisMetricsHistoryLimit-1)
	_, _ = pipe.Exec(ctx)
}

func (m *RedisJobMetrics) TypeSnapshot() map[string]TypeMetricSnapshot {
	if m == nil || m.client == nil {
		return map[string]TypeMetricSnapshot{}
	}
	ctx := context.Background()
	types, err := m.client.SMembers(ctx, m.typeSetKey()).Result()
	if err != nil || len(types) == 0 {
		return map[string]TypeMetricSnapshot{}
	}
	pipe := m.client.Pipeline()
	cmds := make(map[string]*redis.MapStringStringCmd, len(types))
	for _, taskType := range types {
		cmds[taskType] = pipe.HGetAll(ctx, m.countersKey(taskType))
	}
	_, _ = pipe.Exec(ctx)

	out := make(map[string]TypeMetricSnapshot, len(types))
	for _, taskType := range types {
		values, err := cmds[taskType].Result()
		if err != nil {
			continue
		}
		out[taskType] = TypeMetricSnapshot{
			Enqueued:  parseMetricInt(values["enqueued"]),
			Succeeded: parseMetricInt(values["succeeded"]),
			Failed:    parseMetricInt(values["failed"]),
			DLQ:       parseMetricInt(values["dlq"]),
		}
	}
	return out
}

func (m *RedisJobMetrics) RecentExecutions(limit int) []JobExecution {
	if m == nil || m.client == nil {
		return nil
	}
	if limit <= 0 || limit > redisMetricsHistoryLimit {
		limit = redisMetricsHistoryLimit
	}
	ctx := context.Background()
	rows, err := m.client.LRange(ctx, m.historyKey(), 0, int64(limit-1)).Result()
	if err != nil {
		return nil
	}
	out := make([]JobExecution, 0, len(rows))
	for _, row := range rows {
		var exec JobExecution
		if err := json.Unmarshal([]byte(row), &exec); err != nil {
			continue
		}
		out = append(out, exec)
	}
	return out
}

func parseMetricInt(raw string) int64 {
	n, _ := strconv.ParseInt(raw, 10, 64) // empty or malformed counts read as 0
	return n
}

func normalizeTimestamp(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	utc := ts.UTC()
	return &utc
}
