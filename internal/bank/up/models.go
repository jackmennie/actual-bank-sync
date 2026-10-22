package up

type accountsResponse struct {
	Data  []accountResource `json:"data"`
	Links paginationLinks   `json:"links"`
}

type transactionsResponse struct {
	Data  []transactionResource `json:"data"`
	Links paginationLinks       `json:"links"`
}

type paginationLinks struct {
	Next string `json:"next"`
}

type accountResource struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	Attributes accountAttributes `json:"attributes"`
}

type accountAttributes struct {
	DisplayName string  `json:"displayName"`
	AccountType string  `json:"accountType"`
	Balance     balance `json:"balance"`
}

type balance struct {
	CurrencyCode     string `json:"currencyCode"`
	Value            string `json:"value"`
	ValueInBaseUnits int64  `json:"valueInBaseUnits"`
}

type transactionResource struct {
	ID            string                   `json:"id"`
	Attributes    transactionAttributes    `json:"attributes"`
	Relationships transactionRelationships `json:"relationships"`
}

type transactionRelationships struct {
	TransferAccount struct {
		Data *resourceIdentifier `json:"data"`
	} `json:"transferAccount"`
}

type resourceIdentifier struct {
	ID string `json:"id"`
}

type transactionAttributes struct {
	Status      string `json:"status"`
	Description string `json:"description"`
	Message     string `json:"message"`
	Note        *struct {
		Text string `json:"text"`
	} `json:"note"`
	Amount    balance `json:"amount"`
	CreatedAt string  `json:"createdAt"`
	SettledAt string  `json:"settledAt"`
}
