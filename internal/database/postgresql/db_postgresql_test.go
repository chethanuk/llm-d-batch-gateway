/*
Copyright 2026 The llm-d Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package postgresql

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestShortSpanName(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{"SELECT id, tenant_id FROM batch_items WHERE tenant_id = $1 ORDER BY id OFFSET $2 LIMIT $3", "select_batch_items"},
		{"SELECT id, tenant_id, purpose FROM file_items WHERE id = ANY($1) AND tenant_id = $2 ORDER BY id OFFSET $3 LIMIT $4", "select_file_items"},
		{"INSERT INTO batch_items (id, tenant_id, expiry, tags, spec, status) VALUES ($1, $2, $3, $4, $5, $6)", "insert_batch_items"},
		{"INSERT INTO file_items (id, tenant_id) VALUES ($1, $2)", "insert_file_items"},
		{"UPDATE batch_items SET status = $1, tags = $2 WHERE id = $3", "update_batch_items"},
		{"UPDATE file_items SET status = $1 WHERE id = $2", "update_file_items"},
		{"DELETE FROM file_items WHERE id = ANY($1) RETURNING id", "delete_file_items"},
		{"DELETE FROM batch_items WHERE id = ANY($1) RETURNING id", "delete_batch_items"},
		{"", "db_query"},
		{"SELECT", "select"},
		{"COMMIT", "commit"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := shortSpanName(tt.sql); got != tt.want {
				t.Errorf("shortSpanName(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

func TestTokenAfter(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []string
		keyword string
		want    string
	}{
		{"found", []string{"SELECT", "id", "FROM", "batch_items", "WHERE"}, "FROM", "batch_items"},
		{"case insensitive", []string{"delete", "from", "file_items"}, "FROM", "file_items"},
		{"not found", []string{"SELECT", "id"}, "FROM", ""},
		{"keyword at end", []string{"SELECT", "FROM"}, "FROM", ""},
		{"empty", []string{}, "FROM", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenAfter(tt.tokens, tt.keyword); got != tt.want {
				t.Errorf("tokenAfter(%v, %q) = %q, want %q", tt.tokens, tt.keyword, got, tt.want)
			}
		})
	}
}

func TestNewPoolConfig(t *testing.T) {
	const url = "postgres://localhost:5432/batch"
	withCap := func(query string, maxConnections int32) *PostgreSQLConfig {
		return &PostgreSQLConfig{Url: url + query, Pool: PostgreSQLPoolConfig{MaxConnections: maxConnections}}
	}

	tests := []struct {
		name       string
		config     *PostgreSQLConfig
		wantMax    int32
		wantTracer bool
		wantErr    string
	}{
		{name: "zero keeps the pgxpool default", config: withCap("", 0), wantMax: int32(max(4, runtime.NumCPU()))},
		{name: "explicit value is applied", config: withCap("", 7), wantMax: 7},
		{name: "one is raised to two for LISTEN", config: withCap("", 1), wantMax: 2},
		{name: "two is kept", config: withCap("", 2), wantMax: 2},
		{name: "value lowers DSN pool_max_conns", config: withCap("?pool_max_conns=20", 5), wantMax: 5},
		{name: "value raises DSN pool_max_conns", config: withCap("?pool_max_conns=5", 20), wantMax: 20},
		{name: "zero keeps DSN pool_max_conns", config: withCap("?pool_max_conns=20", 0), wantMax: 20},
		{name: "DSN pool_max_conns of one is raised to two", config: withCap("?pool_max_conns=1", 0), wantMax: 2},
		{name: "negative value is rejected", config: withCap("", -1), wantErr: "pool.max_connections"},
		{name: "nil config is rejected", wantErr: "config is nil"},
		{name: "unparsable URL is rejected", config: withCap("?pool_max_conns=abc", 0), wantErr: "failed to parse connection string"},
		{
			name:       "tracing installs a tracer",
			config:     &PostgreSQLConfig{Url: url, EnableTracing: true},
			wantMax:    int32(max(4, runtime.NumCPU())),
			wantTracer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newPoolConfig(context.Background(), tt.config)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("newPoolConfig error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newPoolConfig: %v", err)
			}
			if got.MaxConns != tt.wantMax {
				t.Errorf("MaxConns = %d, want %d", got.MaxConns, tt.wantMax)
			}
			if hasTracer := got.ConnConfig.Tracer != nil; hasTracer != tt.wantTracer {
				t.Errorf("tracer installed = %v, want %v", hasTracer, tt.wantTracer)
			}
		})
	}
}

// TestNewPoolAppliesMaxConnections requires a real PostgreSQL instance (TEST_POSTGRES_URL).
func TestNewPoolAppliesMaxConnections(t *testing.T) {
	url := requirePostgresURL(t)

	tests := []struct {
		name           string
		maxConnections int32
		want           int32
	}{
		{name: "one is raised to two", maxConnections: 1, want: 2},
		{name: "explicit value is applied", maxConnections: 3, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := newPool(context.Background(), &PostgreSQLConfig{
				Url:  url,
				Pool: PostgreSQLPoolConfig{MaxConnections: tt.maxConnections},
			})
			if err != nil {
				t.Fatalf("newPool: %v", err)
			}
			defer pool.Close()
			if got := pool.Stat().MaxConns(); got != tt.want {
				t.Errorf("MaxConns = %d, want %d", got, tt.want)
			}
		})
	}
}
