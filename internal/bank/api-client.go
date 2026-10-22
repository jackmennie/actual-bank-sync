package bank

import (
	"context"
	"time"
)

type BankClient interface {
	GetAccounts(ctx context.Context) ([]Account, error)

	GetTransactions(
		ctx context.Context,
		accountID string,
		since time.Time,
	) ([]Transaction, error)
}
