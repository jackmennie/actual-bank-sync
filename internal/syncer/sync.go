package syncer

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jackmennie/actual-bank-sync/internal/actual"
	"github.com/jackmennie/actual-bank-sync/internal/bank"
	"github.com/jackmennie/actual-bank-sync/internal/config"
)

const checkpointOverlap = 7 * 24 * time.Hour

type Syncer struct {
	actualClient        actual.ActualClient
	bankClients         map[string]bank.BankClient
	mappingsFile        string
	stateFile           string
	initialLookbackDays int
	now                 func() time.Time
	logger              *log.Logger
	verbose             bool
}

type syncState struct {
	LastSynced map[string]time.Time `yaml:"lastSynced"`
}

type Result struct {
	DryRun      bool
	Imported    int
	Updated     int
	WouldAdd    int
	WouldUpdate int
	Accounts    int
	PreviewRows []PreviewRow
}

type PreviewRow struct {
	Action          string
	Provider        string
	BankAccountID   string
	ActualAccountID string
	TransactionID   string
	Date            string
	AmountMinor     int64
	Currency        string
	Payee           string
	Notes           string
	ExistingDate    string
	ExistingAmount  string
	ExistingPayee   string
	ExistingNotes   string
	Diff            string
}

func New(
	actualClient actual.ActualClient,
	bankClients map[string]bank.BankClient,
	mappingsFile string,
	stateFile string,
	initialLookbackDays int,
) *Syncer {
	return &Syncer{
		actualClient:        actualClient,
		bankClients:         bankClients,
		mappingsFile:        mappingsFile,
		stateFile:           stateFile,
		initialLookbackDays: initialLookbackDays,
		now:                 time.Now,
		logger:              log.New(os.Stderr, "", log.LstdFlags),
	}
}

func (s *Syncer) SetVerbose(verbose bool) {
	s.verbose = verbose
}

func (s *Syncer) SetLoggerOutput(output io.Writer) {
	s.logger = log.New(output, "", log.LstdFlags)
}

func (s *Syncer) Run(ctx context.Context, dryRun bool) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(s.stateFile), 0700); err != nil {
		return Result{}, fmt.Errorf("creating sync state directory: %w", err)
	}
	lock, err := os.OpenFile(s.stateFile+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Result{}, fmt.Errorf("opening sync lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return Result{}, fmt.Errorf("another sync process is already running: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	mappings, err := config.LoadAccounts(s.mappingsFile)
	if err != nil {
		return Result{}, fmt.Errorf("loading account mappings: %w", err)
	}
	if len(mappings.Mappings) == 0 {
		return Result{}, fmt.Errorf("no account mappings found; run setup first")
	}
	s.logger.Printf("Starting %s sync for %d mapped account(s)", runMode(dryRun), len(mappings.Mappings))

	state, err := loadState(s.stateFile)
	if err != nil {
		return Result{}, err
	}
	now := s.now().UTC()
	result := Result{DryRun: dryRun}
	orderedMappings := append([]config.AccountMapping(nil), mappings.Mappings...)
	sort.Slice(orderedMappings, func(i, j int) bool {
		if orderedMappings[i].Provider == orderedMappings[j].Provider {
			return orderedMappings[i].BankAccountID < orderedMappings[j].BankAccountID
		}
		return orderedMappings[i].Provider < orderedMappings[j].Provider
	})
	actualAccountByProviderAccount := make(map[string]string, len(orderedMappings))
	for _, mapping := range orderedMappings {
		actualAccountByProviderAccount[mapping.Provider+":"+mapping.BankAccountID] = mapping.ActualAccountID
	}

	for _, mapping := range orderedMappings {
		provider, ok := s.bankClients[mapping.Provider]
		if !ok {
			return result, fmt.Errorf("provider %q for mapped account %q is not configured", mapping.Provider, mapping.BankAccountID)
		}
		if mapping.BankAccountID == "" || mapping.ActualAccountID == "" {
			return result, fmt.Errorf("incomplete account mapping for provider %q", mapping.Provider)
		}

		key := mapping.Provider + ":" + mapping.BankAccountID
		since, ok := state.LastSynced[key]
		if !ok {
			since = now.AddDate(0, 0, -s.initialLookbackDays)
		} else {
			since = since.Add(-checkpointOverlap)
		}
		if s.verbose {
			s.logger.Printf("Checking %s account %s mapped to Actual account %s since %s", mapping.Provider, mapping.BankAccountID, mapping.ActualAccountID, since.Format(time.RFC3339))
		} else {
			s.logger.Printf("Checking %s account %s", mapping.Provider, mapping.BankAccountID)
		}

		transactions, err := provider.GetTransactions(ctx, mapping.BankAccountID, since)
		if err != nil {
			return result, fmt.Errorf("fetching %s transactions for account %q: %w", mapping.Provider, mapping.BankAccountID, err)
		}
		s.logger.Printf("Fetched %d settled transaction(s) from %s account %s", len(transactions), mapping.Provider, mapping.BankAccountID)

		actualTransactions := make([]actual.ImportTransaction, 0, len(transactions))
		for _, transaction := range transactions {
			transferAccountID := ""
			if transaction.TransferAccountID != "" {
				transferAccountID = actualAccountByProviderAccount[mapping.Provider+":"+transaction.TransferAccountID]
			}
			actualTransactions = append(actualTransactions, actual.ImportTransaction{
				Date:              transaction.Date.Format("2006-01-02"),
				Amount:            transaction.Amount,
				PayeeName:         transaction.Description,
				Notes:             transaction.Notes,
				ImportedID:        transaction.ID,
				TransferAccountID: transferAccountID,
			})
		}

		if len(actualTransactions) > 0 {
			if dryRun {
				s.logger.Printf("Previewing %d transaction(s) for Actual account %s", len(actualTransactions), mapping.ActualAccountID)
			} else {
				s.logger.Printf("Importing %d transaction(s) into Actual account %s", len(actualTransactions), mapping.ActualAccountID)
			}
			importResult, err := s.actualClient.ImportTransactions(ctx, mapping.ActualAccountID, actualTransactions, dryRun)
			if err != nil {
				return result, fmt.Errorf("importing transactions into Actual account %q: %w", mapping.ActualAccountID, err)
			}
			if dryRun {
				result.WouldAdd += len(importResult.Added)
				result.WouldUpdate += len(importResult.Updated)
				result.PreviewRows = append(result.PreviewRows, previewRows(mapping, transactions, actualTransactions, importResult.Preview)...)
				s.logger.Printf("Preview result for Actual account %s: %d would add, %d would update", mapping.ActualAccountID, len(importResult.Added), len(importResult.Updated))
			} else {
				result.Imported += len(importResult.Added)
				result.Updated += len(importResult.Updated)
				s.logger.Printf("Import result for Actual account %s: %d added, %d updated", mapping.ActualAccountID, len(importResult.Added), len(importResult.Updated))
			}
		} else {
			s.logger.Printf("No transactions to import for %s account %s", mapping.Provider, mapping.BankAccountID)
		}

		if !dryRun {
			state.LastSynced[key] = now
			if err := saveState(s.stateFile, state); err != nil {
				return result, fmt.Errorf("saving sync checkpoint for %s account %q: %w", mapping.Provider, mapping.BankAccountID, err)
			}
			s.logger.Printf("Saved sync checkpoint for %s account %s", mapping.Provider, mapping.BankAccountID)
		}
		result.Accounts++
	}
	s.logger.Printf("Sync finished: %d account(s) processed", result.Accounts)

	return result, nil
}

func previewRows(mapping config.AccountMapping, bankTransactions []bank.Transaction, imports []actual.ImportTransaction, previews []actual.TransactionPreview) []PreviewRow {
	byImportedID := make(map[string]actual.TransactionPreview, len(previews))
	for _, preview := range previews {
		byImportedID[preview.ImportedID] = preview
	}
	rows := make([]PreviewRow, 0, len(previews))
	for index, imported := range imports {
		preview, ok := byImportedID[imported.ImportedID]
		if !ok || (preview.Action != "added" && preview.Action != "updated") {
			continue
		}
		row := PreviewRow{
			Action:          preview.Action,
			Provider:        mapping.Provider,
			BankAccountID:   mapping.BankAccountID,
			ActualAccountID: mapping.ActualAccountID,
			TransactionID:   imported.ImportedID,
			Date:            imported.Date,
			AmountMinor:     imported.Amount,
			Currency:        bankTransactions[index].Currency,
			Payee:           imported.PayeeName,
			Notes:           imported.Notes,
		}
		if preview.Existing != nil {
			row.ExistingDate = preview.Existing.Date
			row.ExistingAmount = fmt.Sprintf("%.2f", preview.Existing.Amount)
			row.ExistingPayee = preview.Existing.PayeeName
			row.ExistingNotes = preview.Existing.Notes
			row.Diff = describeDiff(row)
		} else {
			row.Diff = "new transaction"
		}
		rows = append(rows, row)
	}
	return rows
}

func describeDiff(row PreviewRow) string {
	var changes []string
	if row.ExistingDate != row.Date {
		changes = append(changes, fmt.Sprintf("date: %q -> %q", row.ExistingDate, row.Date))
	}
	if !amountsEqual(row.ExistingAmount, row.AmountMinor) {
		changes = append(changes, fmt.Sprintf("amount: %s -> %s %s", row.ExistingAmount, formatMinor(row.AmountMinor), row.Currency))
	}
	if row.ExistingPayee != row.Payee {
		changes = append(changes, fmt.Sprintf("payee: %q -> %q", row.ExistingPayee, row.Payee))
	}
	if row.ExistingNotes != row.Notes {
		changes = append(changes, fmt.Sprintf("notes: %q -> %q", row.ExistingNotes, row.Notes))
	}
	return strings.Join(changes, "; ")
}

func amountsEqual(existing string, importedMinor int64) bool {
	var existingAmount float64
	if _, err := fmt.Sscanf(existing, "%f", &existingAmount); err != nil {
		return false
	}
	return math.Abs(existingAmount-float64(importedMinor)/100) < 0.005
}

func formatMinor(amount int64) string {
	return fmt.Sprintf("%.2f", float64(amount)/100)
}

func WritePreviewCSV(writer io.Writer, rows []PreviewRow) error {
	w := csv.NewWriter(writer)
	if err := w.Write([]string{
		"action", "provider", "bank_account_id", "actual_account_id", "transaction_id",
		"up_date", "up_amount", "up_amount_minor", "currency", "up_payee", "up_notes",
		"actual_date", "actual_amount", "actual_payee", "actual_notes", "diff",
	}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write([]string{
			row.Action, row.Provider, row.BankAccountID, row.ActualAccountID, row.TransactionID,
			row.Date, formatMinor(row.AmountMinor), fmt.Sprintf("%d", row.AmountMinor), row.Currency, row.Payee, row.Notes,
			row.ExistingDate, row.ExistingAmount, row.ExistingPayee, row.ExistingNotes, row.Diff,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func WritePreviewCSVFile(path string, rows []PreviewRow) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("creating preview report directory: %w", err)
	}
	file, err := os.CreateTemp(directory, ".actual-up-sync-preview-*.csv")
	if err != nil {
		return fmt.Errorf("creating temporary preview report: %w", err)
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return fmt.Errorf("setting preview report permissions: %w", err)
	}
	if err := WritePreviewCSV(file, rows); err != nil {
		file.Close()
		return fmt.Errorf("writing preview report: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing preview report: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replacing preview report: %w", err)
	}
	return nil
}

func runMode(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "normal"
}

func loadState(path string) (syncState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return syncState{LastSynced: make(map[string]time.Time)}, nil
		}
		return syncState{}, fmt.Errorf("reading sync state: %w", err)
	}
	var state syncState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return syncState{}, fmt.Errorf("parsing sync state: %w", err)
	}
	if state.LastSynced == nil {
		state.LastSynced = make(map[string]time.Time)
	}
	return state, nil
}

func saveState(path string, state syncState) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("encoding sync state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating sync state directory: %w", err)
	}
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0600); err != nil {
		return fmt.Errorf("writing temporary sync state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replacing sync state: %w", err)
	}
	return nil
}
