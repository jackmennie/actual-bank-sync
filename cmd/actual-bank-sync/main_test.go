package main

import "testing"

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
