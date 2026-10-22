package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackmennie/actual-bank-sync/internal/actual"
	"github.com/jackmennie/actual-bank-sync/internal/bank"
	"github.com/jackmennie/actual-bank-sync/internal/config"
)

type testActualClient struct {
	accounts []actual.Account
}

func (c testActualClient) GetAccounts(context.Context) ([]actual.Account, error) {
	return c.accounts, nil
}

func (testActualClient) ImportTransactions(context.Context, string, []actual.ImportTransaction, bool) (actual.ImportResult, error) {
	return actual.ImportResult{}, nil
}

type testBankClient struct {
	accounts []bank.Account
}

func (c testBankClient) GetAccounts(context.Context) ([]bank.Account, error) {
	return c.accounts, nil
}

func (testBankClient) GetTransactions(context.Context, string, time.Time) ([]bank.Transaction, error) {
	return nil, nil
}

func TestRunSavesSelectedAccountMapping(t *testing.T) {
	mappingsPath := filepath.Join(t.TempDir(), "accounts.yaml")
	var output bytes.Buffer
	setup := New(
		testActualClient{accounts: []actual.Account{{ID: "actual-1", Name: "Daily"}}},
		map[string]bank.BankClient{"UP": testBankClient{accounts: []bank.Account{{ID: "up-1", Name: "Spending"}}}},
		mappingsPath,
		strings.NewReader("1\n\n"),
		&output,
	)

	if err := setup.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	mappings, err := config.LoadAccounts(mappingsPath)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(mappings.Mappings) != 1 {
		t.Fatalf("mapping count = %d, want 1", len(mappings.Mappings))
	}
	got := mappings.Mappings[0]
	if got.Provider != "UP" || got.BankAccountID != "up-1" || got.ActualAccountID != "actual-1" {
		t.Fatalf("mapping = %#v, want UP up-1 to actual-1", got)
	}
	if _, err := os.Stat(mappingsPath); err != nil {
		t.Fatalf("mapping file was not written: %v", err)
	}
	if !strings.Contains(output.String(), "1. Daily") {
		t.Fatalf("setup output missing numbered Actual account: %s", output.String())
	}
}

func TestRunEnterSkipsFirstRunMapping(t *testing.T) {
	mappingsPath := filepath.Join(t.TempDir(), "accounts.yaml")
	var output bytes.Buffer
	setup := New(
		testActualClient{accounts: []actual.Account{{ID: "actual-1", Name: "Daily"}}},
		map[string]bank.BankClient{"UP": testBankClient{accounts: []bank.Account{{ID: "up-1", Name: "Spending"}}}},
		mappingsPath,
		strings.NewReader("\n\n"),
		&output,
	)

	if err := setup.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	mappings, err := config.LoadAccounts(mappingsPath)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(mappings.Mappings) != 0 {
		t.Fatalf("mapping count = %d, want blank input to skip", len(mappings.Mappings))
	}
}

func TestRunAgainShowsEditModeWithoutAddWhenNoNewAccounts(t *testing.T) {
	mappingsPath := filepath.Join(t.TempDir(), "accounts.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	var output bytes.Buffer
	setup := New(
		testActualClient{accounts: []actual.Account{{ID: "actual-1", Name: "Daily"}, {ID: "actual-2", Name: "Savings"}}},
		map[string]bank.BankClient{"UP": testBankClient{accounts: []bank.Account{{ID: "up-1", Name: "Spending"}}}},
		mappingsPath,
		strings.NewReader("1\n1\n2\n\n\n"),
		&output,
	)

	if err := setup.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output.String(), "1. Reassign a mapping") || !strings.Contains(output.String(), "2. Remove a mapping") {
		t.Fatalf("edit choices missing: %s", output.String())
	}
	if strings.Contains(output.String(), "3. Add mapping") {
		t.Fatalf("add option shown without new accounts: %s", output.String())
	}
	mappings, err := config.LoadAccounts(mappingsPath)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(mappings.Mappings) != 1 || mappings.Mappings[0].ActualAccountID != "actual-2" {
		t.Fatalf("mappings after reassignment = %#v", mappings.Mappings)
	}
}

func TestRunAgainOffersAddForNewProviderAccount(t *testing.T) {
	mappingsPath := filepath.Join(t.TempDir(), "accounts.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	var output bytes.Buffer
	setup := New(
		testActualClient{accounts: []actual.Account{{ID: "actual-1", Name: "Daily"}, {ID: "actual-2", Name: "Savings"}}},
		map[string]bank.BankClient{"UP": testBankClient{accounts: []bank.Account{{ID: "up-1", Name: "Spending"}, {ID: "up-2", Name: "Saver"}}}},
		mappingsPath,
		strings.NewReader("3\n1\n2\n\n\n"),
		&output,
	)

	if err := setup.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output.String(), "3. Add mapping (1 new account(s) detected)") {
		t.Fatalf("new account add option missing: %s", output.String())
	}
	mappings, err := config.LoadAccounts(mappingsPath)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(mappings.Mappings) != 2 || mappings.Mappings[1].BankAccountID != "up-2" || mappings.Mappings[1].ActualAccountID != "actual-2" {
		t.Fatalf("mappings after adding account = %#v", mappings.Mappings)
	}
}

func TestRunCanCancelAddingMappingAtEitherSelection(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "provider account selection", input: "3\n\n\n\n"},
		{name: "Actual account selection", input: "3\n1\n\n\n\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mappingsPath := filepath.Join(t.TempDir(), "accounts.yaml")
			initial := &config.AccountsConfig{Mappings: []config.AccountMapping{{
				Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
			}}}
			if err := config.SaveAccounts(mappingsPath, initial); err != nil {
				t.Fatalf("SaveAccounts() error = %v", err)
			}
			var output bytes.Buffer
			setup := New(
				testActualClient{accounts: []actual.Account{{ID: "actual-1", Name: "Daily"}, {ID: "actual-2", Name: "Savings"}}},
				map[string]bank.BankClient{"UP": testBankClient{accounts: []bank.Account{{ID: "up-1", Name: "Spending"}, {ID: "up-2", Name: "Saver"}}}},
				mappingsPath,
				strings.NewReader(test.input),
				&output,
			)

			if err := setup.Run(context.Background()); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			mappings, err := config.LoadAccounts(mappingsPath)
			if err != nil {
				t.Fatalf("LoadAccounts() error = %v", err)
			}
			if len(mappings.Mappings) != 1 || mappings.Mappings[0] != initial.Mappings[0] {
				t.Fatalf("mappings after cancel = %#v, want unchanged %#v", mappings.Mappings, initial.Mappings)
			}
			if !strings.Contains(output.String(), "Enter to cancel") {
				t.Fatalf("cancel hint missing: %s", output.String())
			}
		})
	}
}

func TestRunAgainCanRemoveMapping(t *testing.T) {
	mappingsPath := filepath.Join(t.TempDir(), "accounts.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	var output bytes.Buffer
	setup := New(
		testActualClient{accounts: []actual.Account{{ID: "actual-1", Name: "Daily"}}},
		map[string]bank.BankClient{"UP": testBankClient{accounts: []bank.Account{{ID: "up-1", Name: "Spending"}}}},
		mappingsPath,
		strings.NewReader("2\n1\n\n\n"),
		&output,
	)

	if err := setup.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	mappings, err := config.LoadAccounts(mappingsPath)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(mappings.Mappings) != 0 {
		t.Fatalf("mapping count = %d, want removed mapping", len(mappings.Mappings))
	}
}
