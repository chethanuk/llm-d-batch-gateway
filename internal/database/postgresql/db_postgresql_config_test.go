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

// External test package: internal/shared/config imports postgresql, so decoding
// into DBClientConfig from package postgresql would be an import cycle.
package postgresql_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	sharedcfg "github.com/llm-d/llm-d-batch-gateway/internal/shared/config"
)

func TestDBClientConfigPoolMaxConnections(t *testing.T) {
	tests := []struct {
		name            string
		yaml            string
		wantDecodeErr   bool
		wantMax         int32
		wantValidateErr string
	}{
		{
			name: "key absent leaves the cap unset",
			yaml: "type: postgresql\npostgresql: {}\n",
		},
		{
			name: "explicit zero leaves the cap unset",
			yaml: "postgresql:\n  pool:\n    max_connections: 0\n",
		},
		{
			name:    "explicit value reaches the postgres config",
			yaml:    "postgresql:\n  pool:\n    max_connections: 7\n",
			wantMax: 7,
		},
		{
			name:    "one is accepted because the pool floor raises it later",
			yaml:    "postgresql:\n  pool:\n    max_connections: 1\n",
			wantMax: 1,
		},
		{
			name:            "negative value fails validation",
			yaml:            "postgresql:\n  pool:\n    max_connections: -3\n",
			wantMax:         -3,
			wantValidateErr: "pool.max_connections",
		},
		{
			name:          "value overflowing int32 fails to decode",
			yaml:          "postgresql:\n  pool:\n    max_connections: 3000000000\n",
			wantDecodeErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg sharedcfg.DBClientConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			if tt.wantDecodeErr {
				if err == nil {
					t.Fatalf("Unmarshal succeeded with MaxConnections=%d, want a decode error", cfg.PostgreSQLCfg.Pool.MaxConnections)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if got := cfg.PostgreSQLCfg.Pool.MaxConnections; got != tt.wantMax {
				t.Errorf("MaxConnections = %d, want %d", got, tt.wantMax)
			}

			// Url is yaml:"-" and comes from a secret at runtime.
			cfg.PostgreSQLCfg.Url = "postgres://localhost:5432/batch"
			err = cfg.PostgreSQLCfg.Validate()
			switch {
			case tt.wantValidateErr == "" && err != nil:
				t.Errorf("Validate: %v", err)
			case tt.wantValidateErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantValidateErr)):
				t.Errorf("Validate error = %v, want it to contain %q", err, tt.wantValidateErr)
			}
		})
	}
}
