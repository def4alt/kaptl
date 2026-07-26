package nbu_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/def4alt/kaptl/internal/fx/nbu"
)

func TestQuoteConvertsUAHToEURUsingOfficialRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("valcode") != "EUR" || r.URL.Query().Get("date") != "20260724" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"rate":51.0577,"cc":"EUR","exchangedate":"24.07.2026"}]`))
	}))
	defer server.Close()

	client := nbu.NewClient(server.URL, server.Client())
	quote, err := client.Quote(context.Background(), "UAH", "EUR", time.Date(2026, 7, 24, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if quote.Provider != "NBU" || quote.Source != "UAH" || quote.Target != "EUR" {
		t.Fatalf("unexpected quote %#v", quote)
	}
	if quote.Rate.StringFixed(12) != "0.019585684432" {
		t.Fatalf("got rate %s", quote.Rate)
	}
	if got := quote.EffectiveAt.UTC().Format("2006-01-02"); got != "2026-07-24" {
		t.Fatalf("effective date = %s, want 2026-07-24", got)
	}
}

func TestQuoteAcceptsRateExactlySevenCalendarDaysOld(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"rate":51.0577,"cc":"EUR","exchangedate":"17.07.2026"}]`))
	}))
	defer server.Close()

	_, err := nbu.NewClient(server.URL, server.Client()).Quote(context.Background(), "UAH", "EUR", time.Date(2026, 7, 24, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
}

func TestQuoteRejectsRateOlderThanFallbackWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"rate":51.0577,"cc":"EUR","exchangedate":"01.07.2026"}]`))
	}))
	defer server.Close()

	_, err := nbu.NewClient(server.URL, server.Client()).Quote(context.Background(), "UAH", "EUR", time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "older than 7 days") {
		t.Fatalf("error = %v", err)
	}
}

func TestQuoteCrossesForeignCurrencyThroughUAHWithoutBinaryFloats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("valcode") {
		case "USD":
			_, _ = w.Write([]byte(`[{"rate":44.811,"cc":"USD","exchangedate":"24.07.2026"}]`))
		case "EUR":
			_, _ = w.Write([]byte(`[{"rate":51.0577,"cc":"EUR","exchangedate":"24.07.2026"}]`))
		default:
			t.Fatalf("unexpected currency")
		}
	}))
	defer server.Close()

	quote, err := nbu.NewClient(server.URL, server.Client()).Quote(context.Background(), "USD", "EUR", time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if quote.Rate.StringFixed(12) != "0.877654105062" {
		t.Fatalf("got rate %s", quote.Rate)
	}
}
