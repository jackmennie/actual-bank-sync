package syncer

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackmennie/actual-bank-sync/internal/actual"
	"github.com/jackmennie/actual-bank-sync/internal/bank"
	"github.com/jackmennie/actual-bank-sync/internal/config"
)

func TestPreviewReportShowsActualDiffAndUPValues(t *testing.T) {
	imports := []actual.ImportTransaction{{
		Date: "2026-10-20", Amount: -1250, PayeeName: "Cafe Updated", Notes: "New note", ImportedID: "up-tx-1",
	}}
	bankTransactions := []bank.Transaction{{
		ID: "up-tx-1", Currency: "AUD", Date: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
	}}
	previews := []actual.TransactionPreview{{
		ImportedID: "up-tx-1",
		Action:     "updated",
		Existing: &actual.ExistingTransaction{
			Date: "2026-10-19", Amount: -10, PayeeName: "Cafe", Notes: "Old note",
		},
	}}
	rows := previewRows(config.AccountMapping{Provider: "UP", BankAccountID: "up-account", ActualAccountID: "actual-account"}, bankTransactions, imports, previews)
	if len(rows) != 1 || rows[0].Action != "updated" {
		t.Fatalf("preview rows = %#v, want one updated row", rows)
	}
	if rows[0].Diff != `date: "2026-10-19" -> "2026-10-20"; amount: -10.00 -> -12.50 AUD; payee: "Cafe" -> "Cafe Updated"; notes: "Old note" -> "New note"` {
		t.Fatalf("diff = %q", rows[0].Diff)
	}

	var output bytes.Buffer
	if err := WritePreviewCSV(&output, rows); err != nil {
		t.Fatalf("WritePreviewCSV() error = %v", err)
	}
	parsed, err := csv.NewReader(strings.NewReader(output.String())).ReadAll()
	if err != nil {
		t.Fatalf("parsing report CSV: %v", err)
	}
	if len(parsed) != 2 || parsed[1][0] != "updated" || parsed[1][6] != "-12.50" || parsed[1][7] != "-1250" || !strings.Contains(parsed[1][15], "payee:") {
		t.Fatalf("CSV report row = %#v", parsed)
	}
}

func TestPreviewRowsMarksUnmatchedTransactionsAdded(t *testing.T) {
	imports := []actual.ImportTransaction{{Date: "2026-10-20", Amount: 100, ImportedID: "up-new"}}
	transactions := []bank.Transaction{{ID: "up-new", Currency: "AUD"}}
	rows := previewRows(config.AccountMapping{Provider: "UP", BankAccountID: "up-account", ActualAccountID: "actual-account"}, transactions, imports, []actual.TransactionPreview{{ImportedID: "up-new", Action: "added"}})
	if len(rows) != 1 || rows[0].Action != "added" || rows[0].Diff != "new transaction" {
		t.Fatalf("preview rows = %#v, want added transaction", rows)
	}
}

func TestWritePreviewCSVFileUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "preview.csv")
	rows := []PreviewRow{{Action: "added", Provider: "UP", TransactionID: "up-tx", Diff: "new transaction"}}
	if err := WritePreviewCSVFile(path, rows); err != nil {
		t.Fatalf("WritePreviewCSVFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat report: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("report permissions = %04o, want 0600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !bytes.Contains(data, []byte("new transaction")) {
		t.Fatalf("report missing expected content: %s", data)
	}
}

type fakeActualClient struct {
	accountID    string
	transactions []actual.ImportTransaction
	dryRun       bool
	result       actual.ImportResult
	err          error
}

func (c *fakeActualClient) GetAccounts(context.Context) ([]actual.Account, error) {
	return nil, nil
}

func (c *fakeActualClient) ImportTransactions(_ context.Context, accountID string, transactions []actual.ImportTransaction, dryRun bool) (actual.ImportResult, error) {
	c.accountID = accountID
	c.transactions = transactions
	c.dryRun = dryRun
	return c.result, c.err
}

func TestRunDryRunPreviewsWithoutWritingCheckpoint(t *testing.T) {
	dir := t.TempDir()
	mappingsPath := filepath.Join(dir, "accounts.yaml")
	statePath := filepath.Join(dir, "state.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	actualClient := &fakeActualClient{result: actual.ImportResult{
		Added: []string{"preview-add"}, Updated: []string{"preview-update"},
		Preview: []actual.TransactionPreview{{ImportedID: "up-tx", Action: "added"}},
	}}
	bankClient := &fakeBankClient{items: []bank.Transaction{{ID: "up-tx", Amount: 425, Currency: "AUD", Description: "Lunch", Date: time.Now()}}}
	job := New(actualClient, map[string]bank.BankClient{"UP": bankClient}, mappingsPath, statePath, 90)

	result, err := job.Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run(dryRun) error = %v", err)
	}
	if !actualClient.dryRun {
		t.Fatal("Actual import did not receive dry-run flag")
	}
	if !result.DryRun || result.WouldAdd != 1 || result.WouldUpdate != 1 || result.Imported != 0 || result.Updated != 0 {
		t.Fatalf("dry-run result = %#v", result)
	}
	if len(result.PreviewRows) != 1 || result.PreviewRows[0].Action != "added" || result.PreviewRows[0].Payee != "Lunch" {
		t.Fatalf("preview rows = %#v, want added Lunch row", result.PreviewRows)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkpoint was written during dry-run, stat error = %v", err)
	}
}

type fakeBankClient struct {
	accountID      string
	since          time.Time
	accountIDs     []string
	sinceByAccount map[string]time.Time
	items          []bank.Transaction
	itemsByAccount map[string][]bank.Transaction
	err            error
}

func (c *fakeBankClient) GetAccounts(context.Context) ([]bank.Account, error) {
	return nil, nil
}

func (c *fakeBankClient) GetTransactions(_ context.Context, accountID string, since time.Time) ([]bank.Transaction, error) {
	c.accountID = accountID
	c.since = since
	c.accountIDs = append(c.accountIDs, accountID)
	if c.sinceByAccount == nil {
		c.sinceByAccount = make(map[string]time.Time)
	}
	c.sinceByAccount[accountID] = since
	if c.itemsByAccount != nil {
		return c.itemsByAccount[accountID], c.err
	}
	return c.items, c.err
}

func TestRunImportsMappedTransactionsAndWritesCheckpoint(t *testing.T) {
	dir := t.TempDir()
	mappingsPath := filepath.Join(dir, "accounts.yaml")
	statePath := filepath.Join(dir, "state.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}, {
		Provider: "UP", BankAccountID: "up-2", ActualAccountID: "actual-2",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	actualClient := &fakeActualClient{result: actual.ImportResult{Added: []string{"actual-tx"}}}
	var logOutput bytes.Buffer
	bankClient := &fakeBankClient{items: []bank.Transaction{{
		ID: "up-tx", Description: "Cafe", Notes: "Breakfast", Amount: -1200, TransferAccountID: "up-2",
		Currency: "AUD", Date: time.Date(2026, 10, 19, 8, 0, 0, 0, time.FixedZone("AEST", 10*60*60)),
	}}, itemsByAccount: map[string][]bank.Transaction{"up-1": {{
		ID: "up-tx", Description: "Cafe", Notes: "Breakfast", Amount: -1200, TransferAccountID: "up-2",
		Currency: "AUD", Date: time.Date(2026, 10, 19, 8, 0, 0, 0, time.FixedZone("AEST", 10*60*60)),
	}}}}
	job := New(actualClient, map[string]bank.BankClient{"UP": bankClient}, mappingsPath, statePath, 90)
	job.SetLoggerOutput(&logOutput)
	job.now = func() time.Time { return now }

	result, err := job.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(bankClient.accountIDs) != 2 || bankClient.accountIDs[0] != "up-1" || bankClient.accountIDs[1] != "up-2" || actualClient.accountID != "actual-1" {
		t.Fatalf("mapped account IDs: UP=%q Actual=%q", bankClient.accountID, actualClient.accountID)
	}
	if !bankClient.sinceByAccount["up-1"].Equal(now.AddDate(0, 0, -90)) {
		t.Fatalf("initial since = %s, want %s", bankClient.sinceByAccount["up-1"], now.AddDate(0, 0, -90))
	}
	if len(actualClient.transactions) != 1 {
		t.Fatalf("import transaction count = %d, want 1", len(actualClient.transactions))
	}
	imported := actualClient.transactions[0]
	if imported.Date != "2026-10-19" || imported.Amount != -1200 || imported.PayeeName != "Cafe" || imported.Notes != "Breakfast" || imported.ImportedID != "up-tx" {
		t.Fatalf("imported transaction = %#v", imported)
	}
	if imported.TransferAccountID != "actual-2" {
		t.Fatalf("transfer account ID = %q, want actual-2", imported.TransferAccountID)
	}
	if result.Accounts != 2 || result.Imported != 1 {
		t.Fatalf("result = %#v, want two processed accounts and one transaction", result)
	}
	if !strings.Contains(logOutput.String(), "Fetched 1 settled transaction(s)") || !strings.Contains(logOutput.String(), "Importing 1 transaction(s)") {
		t.Fatalf("progress log missing fetch/import messages: %s", logOutput.String())
	}
	state, err := loadState(statePath)
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if !state.LastSynced["UP:up-1"].Equal(now) {
		t.Fatalf("checkpoint = %s, want %s", state.LastSynced["UP:up-1"], now)
	}
}

func TestRunDoesNotAdvanceCheckpointWhenImportFails(t *testing.T) {
	dir := t.TempDir()
	mappingsPath := filepath.Join(dir, "accounts.yaml")
	statePath := filepath.Join(dir, "state.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	actualClient := &fakeActualClient{err: errors.New("helper failed")}
	bankClient := &fakeBankClient{items: []bank.Transaction{{ID: "up-tx", Date: time.Now()}}}
	job := New(actualClient, map[string]bank.BankClient{"UP": bankClient}, mappingsPath, statePath, 90)

	if _, err := job.Run(context.Background(), false); err == nil {
		t.Fatal("Run() error = nil, want import failure")
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file exists after failed import, stat error = %v", err)
	}
}

func TestRunRejectsConcurrentInvocation(t *testing.T) {
	dir := t.TempDir()
	mappingsPath := filepath.Join(dir, "accounts.yaml")
	statePath := filepath.Join(dir, "state", "sync-state.yaml")
	if err := config.SaveAccounts(mappingsPath, &config.AccountsConfig{Mappings: []config.AccountMapping{{
		Provider: "UP", BankAccountID: "up-1", ActualAccountID: "actual-1",
	}}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
		t.Fatalf("creating state directory: %v", err)
	}
	lock, err := os.OpenFile(statePath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("opening lock file: %v", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("acquiring fixture lock: %v", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	job := New(&fakeActualClient{}, map[string]bank.BankClient{"UP": &fakeBankClient{}}, mappingsPath, statePath, 90)
	if _, err := job.Run(context.Background(), false); err == nil || !strings.Contains(err.Error(), "another sync process is already running") {
		t.Fatalf("Run() error = %v, want concurrent-run rejection", err)
	}
}
