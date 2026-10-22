package actual

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetAccountsCallsHelperAndDecodesAccounts(t *testing.T) {
	client, _, requestPath := newTestClient(t, `[{"id":"account-1","name":"Everyday","offbudget":false,"closed":false}]`)

	accounts, err := client.GetAccounts(context.Background())
	if err != nil {
		t.Fatalf("GetAccounts() error = %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != "account-1" || accounts[0].Name != "Everyday" {
		t.Fatalf("GetAccounts() = %#v, want account-1 Everyday", accounts)
	}
	request := readHelperRequest(t, requestPath)
	if request.Action != "accounts" || request.ServerURL != "https://actual.example" || request.Password != "secret" || request.SyncID != "sync-id" {
		t.Fatalf("helper request = %#v, missing expected action/config", request)
	}
}

func TestClientPassesVerboseToHelper(t *testing.T) {
	client, _, requestPath := newTestClient(t, `[]`)
	client.config.Verbose = true
	if _, err := client.GetAccounts(context.Background()); err != nil {
		t.Fatalf("GetAccounts() error = %v", err)
	}
	request := readHelperRequest(t, requestPath)
	if !request.Verbose {
		t.Fatal("helper request Verbose = false, want true")
	}
}

func TestImportTransactionsPassesTransactionsToHelper(t *testing.T) {
	client, _, requestPath := newTestClient(t, `{"added":["transaction-1"],"updated":[],"errors":[]}`)

	result, err := client.ImportTransactions(context.Background(), "account-1", []ImportTransaction{{
		Date:       "2026-10-20",
		Amount:     -1250,
		PayeeName:  "Coffee Shop",
		ImportedID: "up-transaction-1",
	}}, false)
	if err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "transaction-1" {
		t.Fatalf("ImportTransactions() = %#v, want added transaction-1", result)
	}
	request := readHelperRequest(t, requestPath)
	if request.Action != "import-transactions" || request.AccountID != "account-1" || len(request.Transactions) != 1 || request.Transactions[0].ImportedID != "up-transaction-1" {
		t.Fatalf("helper request = %#v, missing transaction import data", request)
	}
}

func TestImportTransactionsPassesDryRunFlagToHelper(t *testing.T) {
	client, _, requestPath := newTestClient(t, `{"added":["preview-id"],"updated":[],"errors":[]}`)
	if _, err := client.ImportTransactions(context.Background(), "account-1", nil, true); err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	request := readHelperRequest(t, requestPath)
	if !request.DryRun {
		t.Fatal("helper request DryRun = false, want true")
	}
}

func TestImportTransactionsReturnsActualImportErrors(t *testing.T) {
	client, _, _ := newTestClient(t, `{"added":[],"updated":[],"errors":[{"message":"invalid account"}]}`)

	_, err := client.ImportTransactions(context.Background(), "account-1", nil, false)
	if err == nil || !strings.Contains(err.Error(), "invalid account") {
		t.Fatalf("ImportTransactions() error = %v, want Actual import error", err)
	}
}

func newTestClient(t *testing.T, output string) (*Client, string, string) {
	t.Helper()

	dir := t.TempDir()
	nodePath := filepath.Join(dir, "node")
	requestPath := filepath.Join(dir, "request.json")
	script := "#!/bin/sh\ncat > '" + requestPath + "'\nprintf '%s\\n' '" + output + "'\n"
	if err := os.WriteFile(nodePath, []byte(script), 0700); err != nil {
		t.Fatalf("writing fake node executable: %v", err)
	}
	helperPath := filepath.Join(dir, "actual-api.mjs")
	if err := os.WriteFile(helperPath, []byte(""), 0600); err != nil {
		t.Fatalf("writing fake helper file: %v", err)
	}
	return NewClient(ClientConfig{
		BaseURL:    "https://actual.example",
		Password:   "secret",
		SyncID:     "sync-id",
		NodeBinary: nodePath,
		HelperPath: helperPath,
	}), helperPath, requestPath
}

func readHelperRequest(t *testing.T, path string) helperRequest {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading helper request: %v", err)
	}
	var request helperRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decoding helper request: %v", err)
	}
	return request
}
