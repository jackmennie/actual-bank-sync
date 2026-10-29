package main

import (
	"reflect"
	"testing"
)

func TestParseGlobalArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantPath  string
		wantArgs  []string
		wantError bool
	}{
		{name: "default config", args: []string{"sync"}, wantPath: "config.yaml", wantArgs: []string{"sync"}},
		{name: "config before command", args: []string{"--config", "/etc/actual-bank-sync/config.yaml", "sync", "--dry-run"}, wantPath: "/etc/actual-bank-sync/config.yaml", wantArgs: []string{"sync", "--dry-run"}},
		{name: "config after command", args: []string{"sync", "--config", "/etc/actual-bank-sync/config.yaml"}, wantPath: "/etc/actual-bank-sync/config.yaml", wantArgs: []string{"sync"}},
		{name: "missing config path", args: []string{"--config"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, gotArgs, err := parseGlobalArgs(test.args)
			if (err != nil) != test.wantError {
				t.Fatalf("parseGlobalArgs() error = %v, wantError %t", err, test.wantError)
			}
			if err == nil && (path != test.wantPath || !reflect.DeepEqual(gotArgs, test.wantArgs)) {
				t.Fatalf("parseGlobalArgs() = (%q, %#v), want (%q, %#v)", path, gotArgs, test.wantPath, test.wantArgs)
			}
		})
	}
}

func TestParseSyncArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantDryRun  bool
		wantVerbose bool
		wantReport  string
		wantErr     bool
	}{
		{name: "normal run"},
		{name: "dry run", args: []string{"--dry-run"}, wantDryRun: true},
		{name: "verbose run", args: []string{"--verbose"}, wantVerbose: true},
		{name: "combined options", args: []string{"--dry-run", "--verbose"}, wantDryRun: true, wantVerbose: true},
		{name: "report", args: []string{"--dry-run", "--report", "preview.csv"}, wantDryRun: true, wantReport: "preview.csv"},
		{name: "report requires dry run", args: []string{"--report", "preview.csv"}, wantErr: true},
		{name: "report requires path", args: []string{"--dry-run", "--report"}, wantErr: true},
		{name: "unknown option", args: []string{"--force"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotDryRun, gotVerbose, gotReport, err := parseSyncArgs(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseSyncArgs() error = %v, wantErr %t", err, test.wantErr)
			}
			if gotDryRun != test.wantDryRun || gotVerbose != test.wantVerbose || gotReport != test.wantReport {
				t.Fatalf("parseSyncArgs() = dryRun:%t verbose:%t report:%q, want dryRun:%t verbose:%t report:%q", gotDryRun, gotVerbose, gotReport, test.wantDryRun, test.wantVerbose, test.wantReport)
			}
		})
	}
}
