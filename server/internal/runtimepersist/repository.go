package runtimepersist

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const ensureTableSQL = `
CREATE TABLE IF NOT EXISTS runtime_snapshots (
    domain text PRIMARY KEY,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);
`

type Repository struct {
	DatabaseURL string
}

func NewRepository(databaseURL string) *Repository {
	return &Repository{DatabaseURL: strings.TrimSpace(databaseURL)}
}

func (r *Repository) Enabled() bool {
	return r != nil && r.DatabaseURL != ""
}

func (r *Repository) Available() bool {
	if !r.Enabled() {
		return false
	}
	_, err := exec.LookPath("psql")
	return err == nil
}

func (r *Repository) EnsureSchema() error {
	if !r.Available() {
		return nil
	}
	_, err := r.runSQL(ensureTableSQL)
	return err
}

func (r *Repository) LoadAll() (map[string]json.RawMessage, error) {
	if err := r.EnsureSchema(); err != nil {
		return nil, err
	}
	if !r.Available() {
		return map[string]json.RawMessage{}, nil
	}

	out, err := r.runSQL("SELECT domain || E'\t' || encode(convert_to(payload::text, 'UTF8'), 'hex') FROM runtime_snapshots ORDER BY domain;")
	if err != nil {
		return nil, err
	}

	result := make(map[string]json.RawMessage)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid runtime snapshot row %q", line)
		}
		payload, err := hex.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode runtime snapshot %s: %w", parts[0], err)
		}
		result[parts[0]] = json.RawMessage(payload)
	}
	return result, nil
}

func (r *Repository) SaveAll(snapshots map[string]json.RawMessage) error {
	if err := r.EnsureSchema(); err != nil {
		return err
	}
	if !r.Available() || len(snapshots) == 0 {
		return nil
	}

	var sql strings.Builder
	sql.WriteString("BEGIN;\n")
	for domain, payload := range snapshots {
		encoded := base64.StdEncoding.EncodeToString(payload)
		sql.WriteString("INSERT INTO runtime_snapshots(domain, payload, updated_at) VALUES ('")
		sql.WriteString(escapeLiteral(domain))
		sql.WriteString("', convert_from(decode('")
		sql.WriteString(encoded)
		sql.WriteString("', 'base64'), 'UTF8')::jsonb, now()) ON CONFLICT(domain) DO UPDATE SET payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at;\n")
	}
	sql.WriteString("COMMIT;\n")

	_, err := r.runSQL(sql.String())
	return err
}

func (r *Repository) runSQL(sql string) (string, error) {
	// Feed SQL through stdin (`psql -f -`) rather than the `-c` argument. Passing
	// large snapshots via `-c` overflows the OS command-line length limit — on
	// Windows this is only ~32KB and fails with "The filename or extension is too
	// long" once the serialized snapshot grows, which previously turned committed
	// mutations into 500s.
	cmd := exec.Command("psql", "-d", r.DatabaseURL, "-v", "ON_ERROR_STOP=1", "-At", "-F", "\t", "-f", "-")
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("runtime snapshot psql failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
