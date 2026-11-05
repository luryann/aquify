package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// orgVersionRetention is how long a stopped process's change-counter rows are kept before they
// are folded into the club's '~retired' row.
const orgVersionRetention = "7 days"

// pruneOrgDataVersions folds old per-process change-counter rows (org_data_versions) into one
// retired row per club and domain. Rows are only added at process restarts, so pruning at
// startup keeps the table bounded. It runs in the background so it never delays boot. A
// failure is logged and ignored: the counters stay correct, only larger.
func pruneOrgDataVersions(pool *pgxpool.Pool, logger *slog.Logger) {
	if pool == nil {
		return
	}
	go pruneOrgDataVersionsNow(pool, logger)
}

func pruneOrgDataVersionsNow(pool *pgxpool.Pool, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var n int
	if err := pool.QueryRow(ctx, `SELECT prune_org_data_versions($1::interval)`, orgVersionRetention).Scan(&n); err != nil {
		logger.Warn("prune org_data_versions failed", "error", err)
		return
	}
	if n > 0 {
		logger.Info("pruned org_data_versions", "folded_rows", n)
	}
}
