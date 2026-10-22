package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jackmennie/actual-bank-sync/internal/actual"
	"github.com/jackmennie/actual-bank-sync/internal/bank"
	"github.com/jackmennie/actual-bank-sync/internal/config"
)

type Setup struct {
	actualClient actual.ActualClient
	bankClients  map[string]bank.BankClient
	mappingsFile string
	input        io.Reader
	output       io.Writer
}

func New(actualClient actual.ActualClient, bankClients map[string]bank.BankClient, mappingsFile string, input io.Reader, output io.Writer) *Setup {
	return &Setup{actualClient: actualClient, bankClients: bankClients, mappingsFile: mappingsFile, input: input, output: output}
}

func (s *Setup) Run(ctx context.Context) error {
	s.printHeader()
	mappings, firstRun, err := s.loadAccounts()
	if err != nil {
		return err
	}
	actualAccounts, err := s.actualClient.GetAccounts(ctx)
	if err != nil {
		return fmt.Errorf("getting Actual accounts: %w", err)
	}
	providers := make([]string, 0, len(s.bankClients))
	for provider := range s.bankClients {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	providerAccounts := make(map[string][]bank.Account, len(providers))
	for _, provider := range providers {
		accounts, err := s.bankClients[provider].GetAccounts(ctx)
		if err != nil {
			return fmt.Errorf("getting %s accounts: %w", provider, err)
		}
		providerAccounts[provider] = accounts
	}
	input := bufio.NewReader(s.input)
	s.printActualAccounts(actualAccounts)
	if firstRun {
		if err := s.assignUnmapped(input, mappings, actualAccounts, providerAccounts, providers); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(s.output, "Existing mappings found; opening edit mode.")
		if err := s.edit(input, mappings, actualAccounts, providerAccounts, providers); err != nil {
			return err
		}
	}

	for {
		s.printReview(mappings, actualAccounts, providerAccounts)
		fmt.Fprint(s.output, "Press Enter to save, or 'e' to edit mappings: ")
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			return fmt.Errorf("reading review choice: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "s", "save":
			if err := config.SaveAccounts(s.mappingsFile, mappings); err != nil {
				return fmt.Errorf("saving account mappings: %w", err)
			}
			fmt.Fprintf(s.output, "Saved %d account mapping(s) to %s\n", len(mappings.Mappings), s.mappingsFile)
			return nil
		case "e", "edit":
			if err := s.edit(input, mappings, actualAccounts, providerAccounts, providers); err != nil {
				return err
			}
		default:
			fmt.Fprintln(s.output, "Enter to save or 'e' to edit.")
		}
	}
}

func (s *Setup) assignUnmapped(input *bufio.Reader, mappings *config.AccountsConfig, actuals []actual.Account, banks map[string][]bank.Account, providers []string) error {
	for _, provider := range providers {
		for _, bankAccount := range banks[provider] {
			if mappingIndex(mappings.Mappings, provider, bankAccount.ID) >= 0 {
				continue
			}
			fmt.Fprintf(s.output, "\n%s account: %s (ID: %s)\n", provider, bankAccount.Name, bankAccount.ID)
			for i, account := range actuals {
				fmt.Fprintf(s.output, "  %d. %s (ID: %s)\n", i+1, account.Name, account.ID)
			}
			fmt.Fprint(s.output, "Map to an Actual account (Enter to skip): ")
			selection, err := s.readSelection(input, len(actuals), true)
			if err != nil {
				return fmt.Errorf("selecting Actual account for %s: %w", bankAccount.Name, err)
			}
			if selection > 0 {
				mappings.Mappings = append(mappings.Mappings, config.AccountMapping{Provider: provider, BankAccountID: bankAccount.ID, ActualAccountID: actuals[selection-1].ID})
			}
		}
	}
	return nil
}

func (s *Setup) edit(input *bufio.Reader, mappings *config.AccountsConfig, actuals []actual.Account, banks map[string][]bank.Account, providers []string) error {
	for {
		newAccounts := getUnmapped(mappings.Mappings, banks, providers)
		s.printReview(mappings, actuals, banks)
		fmt.Fprintln(s.output, "Edit mappings:")
		fmt.Fprintln(s.output, "  1. Reassign a mapping")
		fmt.Fprintln(s.output, "  2. Remove a mapping")
		if len(newAccounts) > 0 {
			fmt.Fprintf(s.output, "  3. Add mapping (%d new account(s) detected)\n", len(newAccounts))
		}
		fmt.Fprint(s.output, "Choose an action (Enter to finish): ")
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			return fmt.Errorf("reading edit action: %w", err)
		}
		switch strings.TrimSpace(line) {
		case "", "0":
			return nil
		case "1":
			index, err := s.selectMapping(input, mappings.Mappings, banks)
			if err != nil {
				return err
			}
			if index < 0 {
				continue
			}
			for i, account := range actuals {
				fmt.Fprintf(s.output, "  %d. %s (ID: %s)\n", i+1, account.Name, account.ID)
			}
			fmt.Fprint(s.output, "Choose the new Actual account: ")
			selection, err := s.readSelection(input, len(actuals), false)
			if err != nil {
				return fmt.Errorf("selecting Actual account: %w", err)
			}
			mappings.Mappings[index].ActualAccountID = actuals[selection-1].ID
		case "2":
			index, err := s.selectMapping(input, mappings.Mappings, banks)
			if err != nil {
				return err
			}
			if index >= 0 {
				mappings.Mappings = append(mappings.Mappings[:index], mappings.Mappings[index+1:]...)
			}
		case "3":
			if len(newAccounts) == 0 {
				fmt.Fprintln(s.output, "No new accounts are available to map.")
				continue
			}
			if err := s.addMapping(input, mappings, actuals, newAccounts); err != nil {
				return err
			}
		default:
			fmt.Fprintln(s.output, "Choose one of the listed actions.")
		}
	}
}

func (s *Setup) selectMapping(input *bufio.Reader, mappings []config.AccountMapping, banks map[string][]bank.Account) (int, error) {
	if len(mappings) == 0 {
		fmt.Fprintln(s.output, "There are no mappings to edit.")
		return -1, nil
	}
	for i, mapping := range mappings {
		fmt.Fprintf(s.output, "  %d. %s: %s -> Actual ID %s\n", i+1, mapping.Provider, lookupBankName(banks[mapping.Provider], mapping.BankAccountID), mapping.ActualAccountID)
	}
	fmt.Fprint(s.output, "Choose a mapping (Enter to cancel): ")
	selection, err := s.readSelection(input, len(mappings), true)
	if err != nil || selection == 0 {
		return -1, err
	}
	return selection - 1, nil
}

func (s *Setup) addMapping(input *bufio.Reader, mappings *config.AccountsConfig, actuals []actual.Account, newAccounts []providerAccount) error {
	for i, account := range newAccounts {
		fmt.Fprintf(s.output, "  %d. %s: %s (ID: %s)\n", i+1, account.Provider, account.Account.Name, account.Account.ID)
	}
	fmt.Fprint(s.output, "Choose a new provider account to map (Enter to cancel): ")
	selection, err := s.readSelection(input, len(newAccounts), true)
	if err != nil {
		return fmt.Errorf("selecting provider account: %w", err)
	}
	if selection == 0 {
		return nil
	}
	selected := newAccounts[selection-1]
	for i, account := range actuals {
		fmt.Fprintf(s.output, "  %d. %s (ID: %s)\n", i+1, account.Name, account.ID)
	}
	fmt.Fprint(s.output, "Choose the Actual account (Enter to cancel): ")
	actualSelection, err := s.readSelection(input, len(actuals), true)
	if err != nil {
		return fmt.Errorf("selecting Actual account: %w", err)
	}
	if actualSelection == 0 {
		return nil
	}
	mappings.Mappings = append(mappings.Mappings, config.AccountMapping{Provider: selected.Provider, BankAccountID: selected.Account.ID, ActualAccountID: actuals[actualSelection-1].ID})
	return nil
}

func (s *Setup) readSelection(input *bufio.Reader, max int, allowBlank bool) (int, error) {
	for {
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			return 0, err
		}
		value := strings.TrimSpace(line)
		if value == "" && allowBlank {
			return 0, nil
		}
		selection, parseErr := strconv.Atoi(value)
		if parseErr == nil && selection >= 1 && selection <= max {
			return selection, nil
		}
		if allowBlank {
			fmt.Fprintf(s.output, "Enter a number from 1 to %d, or press Enter to skip: ", max)
		} else {
			fmt.Fprintf(s.output, "Enter a number from 1 to %d: ", max)
		}
		if err != nil {
			return 0, err
		}
	}
}

func (s *Setup) printReview(mappings *config.AccountsConfig, actuals []actual.Account, banks map[string][]bank.Account) {
	fmt.Fprintln(s.output, "\nCurrent account mappings:")
	if len(mappings.Mappings) == 0 {
		fmt.Fprintln(s.output, "  (none)")
		return
	}
	for i, mapping := range mappings.Mappings {
		fmt.Fprintf(s.output, "  %d. %s: %s -> %s\n", i+1, mapping.Provider, lookupBankName(banks[mapping.Provider], mapping.BankAccountID), lookupActualName(actuals, mapping.ActualAccountID))
	}
}

func (s *Setup) printActualAccounts(accounts []actual.Account) {
	fmt.Fprintln(s.output, "\nFound accounts in Actual:")
	for i, account := range accounts {
		fmt.Fprintf(s.output, "  %d. %s (ID: %s)\n", i+1, account.Name, account.ID)
	}
}

func (s *Setup) loadAccounts() (*config.AccountsConfig, bool, error) {
	accounts, err := config.LoadAccounts(s.mappingsFile)
	if err == nil {
		return accounts, false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return config.NewAccountsConfig(), true, nil
	}
	return nil, false, err
}

func (s *Setup) printHeader() {
	fmt.Fprintln(s.output, "Actual Bank Sync Setup")
	fmt.Fprintln(s.output, "======================")
	fmt.Fprintln(s.output)
}

type providerAccount struct {
	Provider string
	Account  bank.Account
}

func getUnmapped(mappings []config.AccountMapping, banks map[string][]bank.Account, providers []string) []providerAccount {
	var result []providerAccount
	for _, provider := range providers {
		for _, account := range banks[provider] {
			if mappingIndex(mappings, provider, account.ID) < 0 {
				result = append(result, providerAccount{Provider: provider, Account: account})
			}
		}
	}
	return result
}

func mappingIndex(mappings []config.AccountMapping, provider, bankID string) int {
	for i, mapping := range mappings {
		if mapping.Provider == provider && mapping.BankAccountID == bankID {
			return i
		}
	}
	return -1
}

func lookupBankName(accounts []bank.Account, id string) string {
	for _, account := range accounts {
		if account.ID == id {
			return account.Name
		}
	}
	return "unknown account (" + id + ")"
}

func lookupActualName(accounts []actual.Account, id string) string {
	for _, account := range accounts {
		if account.ID == id {
			return account.Name
		}
	}
	return "unknown account (" + id + ")"
}
