package actual

type Account struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OffBudget bool   `json:"offbudget"`
	Closed    bool   `json:"closed"`
}

type ImportTransaction struct {
	Date              string `json:"date"`
	Amount            int64  `json:"amount"`
	PayeeName         string `json:"payee_name,omitempty"`
	Notes             string `json:"notes,omitempty"`
	ImportedID        string `json:"imported_id"`
	TransferAccountID string `json:"transferAccountId,omitempty"`
}

type ImportResult struct {
	Added   []string             `json:"added"`
	Updated []string             `json:"updated"`
	Errors  []any                `json:"errors"`
	Preview []TransactionPreview `json:"preview"`
}

type TransactionPreview struct {
	ImportedID string               `json:"importedId"`
	Action     string               `json:"action"`
	Existing   *ExistingTransaction `json:"existing,omitempty"`
}

type ExistingTransaction struct {
	Date      string  `json:"date"`
	Amount    float64 `json:"amount"`
	PayeeName string  `json:"payee_name"`
	Notes     string  `json:"notes"`
}
