package core

import (
	"reflect"
	"testing"
)

func TestParseRunOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runOptions
		wantErr bool
	}{
		{
			name: "only port",
			args: []string{"5844"},
			want: runOptions{corePort: 5844},
		},
		{
			name: "port with peers and config",
			args: []string{"-config", "6000", "peerA", "peerB"},
			want: runOptions{
				corePort:   6000,
				loadConfig: true,
				peers:      []string{"peerA", "peerB"},
			},
		},
		{
			name: "ui flag after port",
			args: []string{"7000", "-ui", "9000"},
			want: runOptions{
				corePort: 7000,
				uiPort:   9000,
			},
		},
		{
			name: "ui flag before port with equals",
			args: []string{"-ui=7777", "8000"},
			want: runOptions{
				corePort: 8000,
				uiPort:   7777,
			},
		},
		{
			name:    "missing port",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "ui flag without value",
			args:    []string{"6000", "-ui"},
			wantErr: true,
		},
		{
			name:    "invalid port value",
			args:    []string{"notaport"},
			wantErr: true,
		},
		{
			name:    "invalid ui port value",
			args:    []string{"6000", "-ui", "bad"},
			wantErr: true,
		},
		{
			name:    "unknown flag",
			args:    []string{"6000", "-x"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRunOptions(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
