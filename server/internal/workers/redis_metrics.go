package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	logger *slog.Logger
}

func NewRedisJobMetrics(client redis.UniversalClient, prefix string) *RedisJobMetrics {
	if prefix == "" {
		prefix = "aquify:jobs:metrics"
	}
	return &RedisJobMetrics{client: client, prefix: prefix, logger: slog.Default()}
}

// warnLoss records a metrics write/read that failed, so a Redis outage that
// makes job counters and history under-report is visible in the logs.
func (m *RedisJobMetrics) warnLoss(op string, err error, attrs ...any) {
	logger := m.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("workers: redis job metrics unavailable; data dropped", append([]any{"op", op, "error", err}, attrs...)...)
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
	if _, err := pipe.Exec(ctx); err != nil {
		m.warnLoss("increment_counter", err, "task_type", taskType, "field", field)
	}
}

func (m *RedisJobMetrics) appendHistory(ctx context.Context, exec JobExecution) {
	if m == nil || m.client == nil {
		return
	}
	raw, err := json.Marshal(exec)
	if err != nil {
		m.warnLoss("marshal_history", err, "task_type", exec.Type)
		return
	}
	pipe := m.client.TxPipeline()
	pipe.LPush(ctx, m.historyKey(), raw)
	pipe.LTrim(ctx, m.historyKey(), 0, redisMetricsHistoryLimit-1)
	if _, err := pipe.Exec(ctx); err != nil {
		m.warnLoss("append_history", err, "task_type", exec.Type)
	}
}

func (m *RedisJobMetrics) TypeSnapshot() map[string]TypeMetricSnapshot {
	if m == nil || m.client == nil {
		return map[string]TypeMetricSnapshot{}
	}
	ctx := context.Background()
	types, err := m.client.SMembers(ctx, m.typeSetKey()).Result()
	if err != nil {
		m.warnLoss("type_snapshot_types", err)
		return map[string]TypeMetricSnapshot{}
	}
	if len(types) == 0 {
		return map[string]TypeMetricSnapshot{}
	}
	pipe := m.client.Pipeline()
	cmds := make(map[string]*redis.MapStringStringCmd, len(types))
	for _, taskType := range types {
		cmds[taskType] = pipe.HGetAll(ctx, m.countersKey(taskType))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		m.warnLoss("type_snapshot_counters", err)
	}

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
		m.warnLoss("recent_executions", err)
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
