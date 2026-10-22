package bank

import "time"

type Account struct {
	ID   string
	Name string
}

type Transaction struct {
	ID                string
	AccountID         string
	TransferAccountID string
	Description       string
	Notes             string
	Amount            int64
	Currency          string
	Date              time.Time
}
