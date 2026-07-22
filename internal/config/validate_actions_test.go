package config

import (
	"strings"
	"testing"
)

func TestValidateActions(t *testing.T) {
	tests := []struct {
		name    string
		action  *Action
		wantErr string // substring; "" means no error
	}{
		{
			name:   "single statement is valid",
			action: &Action{Kind: "sql", Source: "db", Statement: "UPDATE t SET x=1"},
		},
		{
			name:   "statements batch is valid",
			action: &Action{Kind: "sql", Source: "db", Statements: []string{"UPDATE t SET x=1", "INSERT INTO audit VALUES (1)"}},
		},
		{
			name:    "both statement and statements is rejected",
			action:  &Action{Kind: "sql", Source: "db", Statement: "UPDATE t SET x=1", Statements: []string{"INSERT INTO audit VALUES (1)"}},
			wantErr: "not both",
		},
		{
			name:    "neither is rejected",
			action:  &Action{Kind: "sql", Source: "db"},
			wantErr: "needs a statement",
		},
		{
			name:   "non-sql action is not checked",
			action: &Action{Kind: "exec", Cmd: "./script.sh"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Actions: map[string]*Action{"act": tt.action}}
			err := c.ValidateActions()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
