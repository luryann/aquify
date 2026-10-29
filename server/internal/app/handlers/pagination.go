package handlers

import (
	"net/http"

	"github.com/dareaquatics/aquify/server/internal/app/httpx"
)

const (
	defaultListLimit = 100
	maxListLimit     = 200
)

type listPagination struct {
	Limit  int
	Offset int
}

type listPaginationOptions struct {
	DefaultLimit int
	MaxLimit     int
}

func parseListPagination(r *http.Request, opts listPaginationOptions) listPagination {
	defaultLimit := opts.DefaultLimit
	if defaultLimit <= 0 {
		defaultLimit = defaultListLimit
	}
	maxLimit := opts.MaxLimit
	if maxLimit <= 0 {
		maxLimit = maxListLimit
	}

	query := r.URL.Query()
	limit := parsePositiveInt(query.Get("limit"), defaultLimit)
	if limit > maxLimit {
		limit = maxLimit
	}

	return listPagination{
		Limit:  limit,
		Offset: parseNonNegativeInt(query.Get("offset"), 0),
	}
}

func paginateSlice[T any](items []T, pagination listPagination) ([]T, int) {
	total := len(items)
	if items == nil {
		items = []T{}
	}
	if pagination.Offset >= total {
		return []T{}, total
	}

	end := pagination.Offset + pagination.Limit
	if end > total {
		end = total
	}
	return items[pagination.Offset:end], total
}

func parsePositiveInt(raw string, fallback int) int {
	value := parseNonNegativeInt(raw, fallback)
	if value <= 0 {
		return fallback
	}
	return value
}

func parseNonNegativeInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return fallback
		}
		value = value*10 + int(ch-'0')
	}
	return value
}

// writePagedList answers 200 with the standard list envelope (status, count,
// total, limit, offset) plus the page of items under key.
func writePagedList[T any](w http.ResponseWriter, key string, items []T, total int, p listPagination) {
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"count":  len(items),
		"total":  total,
		"limit":  p.Limit,
		"offset": p.Offset,
		key:      items,
	})
}
