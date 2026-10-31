package up

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetTransactionsPaginatesAndNormalizesSettledTransactions(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer test-token", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/accounts/account-1/transactions" {
			if got := r.URL.Query().Get("filter[status]"); got != "SETTLED" {
				t.Errorf("status filter = %q, want SETTLED", got)
			}
			if got := r.URL.Query().Get("filter[since]"); got != "2026-10-01T00:00:00Z" {
				t.Errorf("since filter = %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"up-tx-1","attributes":{"status":"SETTLED","description":"Cafe","message":"Breakfast","amount":{"currencyCode":"AUD","valueInBaseUnits":-1250},"createdAt":"2026-10-03T08:00:00Z","settledAt":"2026-10-06T09:00:00Z"},"relationships":{"transferAccount":{"data":{"id":"account-2"}}}}],"links":{"next":"` + server.URL + `/page-2"}}`))
			return
		}
		if r.URL.Path == "/page-2" {
			_, _ = w.Write([]byte(`{"data":[{"id":"up-tx-2","attributes":{"status":"SETTLED","description":"Market","note":{"text":"Groceries"},"amount":{"currencyCode":"AUD","valueInBaseUnits":3200},"createdAt":"2026-10-03T08:00:00Z","settledAt":null}}],"links":{"next":null}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newClient("test-token", server.Client(), server.URL)
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	transactions, err := client.GetTransactions(context.Background(), "account-1", since)
	if err != nil {
		t.Fatalf("GetTransactions() error = %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("transaction count = %d, want 2", len(transactions))
	}
	first := transactions[0]
	if first.ID != "up-tx-1" || first.Amount != -1250 || first.Currency != "AUD" || first.Description != "Cafe" || first.Notes != "Breakfast" || first.TransferAccountID != "account-2" {
		t.Fatalf("first transaction = %#v", first)
	}
	if got := first.Date.Format(time.RFC3339); got != "2026-10-03T08:00:00Z" {
		t.Fatalf("transaction date = %s, want createdAt rather than settledAt", got)
	}
	second := transactions[1]
	if second.Notes != "Groceries" || second.Date.Format("2006-01-02") != "2026-10-03" {
		t.Fatalf("second transaction = %#v", second)
	}
}

func TestGetTransactionsRejectsCrossOriginPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"links":{"next":"https://attacker.example/collect"}}`))
	}))
	defer server.Close()

	client := newClient("test-token", server.Client(), server.URL)
	_, err := client.GetTransactions(context.Background(), "account-1", time.Now())
	if err == nil || err.Error() != "getting UP transactions for account account-1: refusing UP pagination URL outside API origin" {
		t.Fatalf("GetTransactions() error = %v, want cross-origin pagination rejection", err)
	}
}
