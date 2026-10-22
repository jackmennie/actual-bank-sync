package up

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackmennie/actual-bank-sync/internal/bank"
)

type UpClient struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

func NewClient(apiKey string) *UpClient {
	return newClient(apiKey, http.DefaultClient, "https://api.up.com.au/api/v1")
}

func newClient(apiKey string, httpClient *http.Client, baseURL string) *UpClient {
	return &UpClient{
		httpClient: httpClient,
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (c *UpClient) GetAccounts(ctx context.Context) ([]bank.Account, error) {
	pageURL := c.baseURL + "/accounts"
	accounts := make([]bank.Account, 0)
	for pageURL != "" {
		var response accountsResponse
		if err := c.getPage(ctx, pageURL, &response); err != nil {
			return nil, fmt.Errorf("getting UP accounts: %w", err)
		}
		for _, account := range response.Data {
			accounts = append(accounts, bank.Account{ID: account.ID, Name: account.Attributes.DisplayName})
		}
		pageURL = response.Links.Next
	}
	return accounts, nil
}

func (c *UpClient) GetTransactions(
	ctx context.Context,
	accountID string,
	since time.Time,
) ([]bank.Transaction, error) {
	endpoint, err := url.Parse(c.baseURL + "/accounts/" + url.PathEscape(accountID) + "/transactions")
	if err != nil {
		return nil, fmt.Errorf("building UP transactions URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("filter[status]", "SETTLED")
	query.Set("filter[since]", since.UTC().Format(time.RFC3339))
	endpoint.RawQuery = query.Encode()

	transactions := make([]bank.Transaction, 0)
	pageURL := endpoint.String()
	for pageURL != "" {
		var response transactionsResponse
		if err := c.getPage(ctx, pageURL, &response); err != nil {
			return nil, fmt.Errorf("getting UP transactions for account %s: %w", accountID, err)
		}
		for _, transaction := range response.Data {
			attributes := transaction.Attributes
			if attributes.Status != "SETTLED" {
				continue
			}
			dateValue := attributes.SettledAt
			if dateValue == "" {
				dateValue = attributes.CreatedAt
			}
			date, err := time.Parse(time.RFC3339, dateValue)
			if err != nil {
				return nil, fmt.Errorf("parsing UP transaction %s date: %w", transaction.ID, err)
			}
			notes := attributes.Message
			if attributes.Note != nil && attributes.Note.Text != "" {
				notes = attributes.Note.Text
			}
			transferAccountID := ""
			if transferAccount := transaction.Relationships.TransferAccount.Data; transferAccount != nil {
				transferAccountID = transferAccount.ID
			}
			transactions = append(transactions, bank.Transaction{
				ID:                transaction.ID,
				AccountID:         accountID,
				TransferAccountID: transferAccountID,
				Description:       attributes.Description,
				Notes:             notes,
				Amount:            attributes.Amount.ValueInBaseUnits,
				Currency:          attributes.Amount.CurrencyCode,
				Date:              date,
			})
		}
		pageURL = response.Links.Next
	}
	return transactions, nil
}

func (c *UpClient) getPage(ctx context.Context, pageURL string, destination any) error {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("invalid UP base URL: %w", err)
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return fmt.Errorf("invalid UP pagination URL: %w", err)
	}
	if !parsed.IsAbs() {
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme != base.Scheme || parsed.Host != base.Host {
		return fmt.Errorf("refusing UP pagination URL outside API origin")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("requesting UP API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("UP API returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(destination); err != nil {
		return fmt.Errorf("decoding UP API response: %w", err)
	}
	return nil
}

var _ bank.BankClient = (*UpClient)(nil)
