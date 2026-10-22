package actual

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type ActualClient interface {
	GetAccounts(ctx context.Context) ([]Account, error)
	ImportTransactions(
		ctx context.Context,
		accountID string,
		transactions []ImportTransaction,
		dryRun bool,
	) (ImportResult, error)
}

type ClientConfig struct {
	BaseURL            string
	Password           string
	SyncID             string
	EncryptionPassword string
	DataDir            string
	NodeBinary         string
	HelperPath         string
	Verbose            bool
}

type Client struct {
	config ClientConfig
}

func NewClient(config ClientConfig) *Client {
	if config.NodeBinary == "" {
		config.NodeBinary = "node"
	}
	if config.HelperPath == "" {
		config.HelperPath = filepath.Join("helper", "actual-api.mjs")
	}
	if config.DataDir == "" {
		config.DataDir = filepath.Join("data", "actual")
	}

	return &Client{config: config}
}

var _ ActualClient = (*Client)(nil)

func (c *Client) GetAccounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	if err := c.call(ctx, helperRequest{Action: "accounts"}, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (c *Client) ImportTransactions(
	ctx context.Context,
	accountID string,
	transactions []ImportTransaction,
	dryRun bool,
) (ImportResult, error) {
	var result ImportResult
	err := c.call(ctx, helperRequest{
		Action:       "import-transactions",
		AccountID:    accountID,
		Transactions: transactions,
		DryRun:       dryRun,
	}, &result)
	if err == nil && len(result.Errors) > 0 {
		return result, fmt.Errorf("Actual transaction import returned errors: %v", result.Errors)
	}
	return result, err
}

type helperRequest struct {
	Action             string              `json:"action"`
	ServerURL          string              `json:"serverURL"`
	Password           string              `json:"password"`
	SyncID             string              `json:"syncId"`
	EncryptionPassword string              `json:"encryptionPassword,omitempty"`
	DataDir            string              `json:"dataDir"`
	Verbose            bool                `json:"verbose,omitempty"`
	AccountID          string              `json:"accountId,omitempty"`
	Transactions       []ImportTransaction `json:"transactions,omitempty"`
	DryRun             bool                `json:"dryRun,omitempty"`
}

type helperResponse struct {
	Error string `json:"error"`
}

func (c *Client) call(ctx context.Context, request helperRequest, result any) error {
	request.ServerURL = strings.TrimRight(c.config.BaseURL, "/")
	request.Password = c.config.Password
	request.SyncID = c.config.SyncID
	request.EncryptionPassword = c.config.EncryptionPassword
	request.DataDir = c.config.DataDir
	request.Verbose = c.config.Verbose
	if request.DataDir != "" {
		dataDir, err := filepath.Abs(request.DataDir)
		if err != nil {
			return fmt.Errorf("resolving Actual data directory: %w", err)
		}
		request.DataDir = dataDir
	}

	input, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encoding Actual helper request: %w", err)
	}

	helperPath, err := filepath.Abs(c.config.HelperPath)
	if err != nil {
		return fmt.Errorf("resolving Actual helper path: %w", err)
	}

	command := exec.CommandContext(ctx, c.config.NodeBinary, helperPath)
	command.Dir = filepath.Dir(helperPath)
	command.Stdin = bytes.NewReader(input)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		var response helperResponse
		if json.Unmarshal(stdout.Bytes(), &response) == nil && response.Error != "" {
			return fmt.Errorf("Actual API helper: %s", response.Error)
		}
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("running Actual API helper: %w: %s", err, message)
		}
		return fmt.Errorf("running Actual API helper: %w", err)
	}

	if err := json.Unmarshal(stdout.Bytes(), result); err != nil {
		return fmt.Errorf("decoding Actual API helper response: %w", err)
	}
	return nil
}
