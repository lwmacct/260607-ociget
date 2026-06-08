package config

import (
	"testing"
	"time"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

var helper = cfgm.ConfigTestHelper[Config]{
	ExamplePath: "config/config.example.yaml",
	ConfigPath:  "config/config.yaml",
}

func TestWriteExample(t *testing.T)    { helper.WriteExampleFile(t, DefaultConfig()) }
func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }

func TestServerDownloadCacheValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerDownloadCache
		wantErr bool
	}{
		{
			name: "disabled",
			cfg:  ServerDownloadCache{Enabled: false},
		},
		{
			name:    "enabled missing dir",
			cfg:     ServerDownloadCache{Enabled: true, TTL: "1h"},
			wantErr: true,
		},
		{
			name:    "invalid ttl",
			cfg:     ServerDownloadCache{Enabled: true, Dir: ".cache", TTL: "soon"},
			wantErr: true,
		},
		{
			name:    "negative ttl",
			cfg:     ServerDownloadCache{Enabled: true, Dir: ".cache", TTL: "-1h"},
			wantErr: true,
		},
		{
			name: "valid",
			cfg:  ServerDownloadCache{Enabled: true, Dir: ".cache", TTL: "168h"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestServerDownloadCacheTTLDuration(t *testing.T) {
	got, err := (ServerDownloadCache{TTL: "2h"}).TTLDuration()
	if err != nil {
		t.Fatalf("TTLDuration() unexpected error: %v", err)
	}
	if got != 2*time.Hour {
		t.Fatalf("TTLDuration() = %v, want 2h", got)
	}

	_, err = (ServerDownloadCache{TTL: "bad"}).TTLDuration()
	if err == nil {
		t.Fatalf("TTLDuration() expected error")
	}
}
