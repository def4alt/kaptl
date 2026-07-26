package nbu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/def4alt/kaptl/internal/fx"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/shopspring/decimal"
)

const ProviderName = "NBU"

type Client struct {
	baseURL string
	http    *http.Client
	now     func() time.Time
}

type responseRow struct {
	Rate         json.Number `json:"rate"`
	Currency     string      `json:"cc"`
	ExchangeDate string      `json:"exchangedate"`
}

func NewClient(baseURL string, client *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: client, now: time.Now}
}

func (c *Client) Name() string { return ProviderName }

func (c *Client) Quote(ctx context.Context, source, target string, at time.Time) (fx.Quote, error) {
	source, err := money.NormalizeCurrency(source)
	if err != nil {
		return fx.Quote{}, err
	}
	target, err = money.NormalizeCurrency(target)
	if err != nil {
		return fx.Quote{}, err
	}
	if target != "EUR" {
		return fx.Quote{}, fmt.Errorf("NBU adapter currently supports EUR as target, got %s", target)
	}
	if source == target {
		return fx.Quote{Source: source, Target: target, Rate: decimal.NewFromInt(1), EffectiveAt: at, ObservedAt: c.now(), Provider: ProviderName}, nil
	}

	eurRate, effectiveAt, err := c.rate(ctx, "EUR", at)
	if err != nil {
		return fx.Quote{}, err
	}
	rate := decimal.NewFromInt(1).Div(eurRate)
	if source != "UAH" {
		sourceRate, sourceEffectiveAt, err := c.rate(ctx, source, at)
		if err != nil {
			return fx.Quote{}, err
		}
		if !sourceEffectiveAt.Equal(effectiveAt) {
			return fx.Quote{}, fmt.Errorf("NBU quote dates differ for %s and EUR", source)
		}
		rate = sourceRate.Div(eurRate)
	}

	return fx.Quote{
		Source: source, Target: target, Rate: rate,
		EffectiveAt: effectiveAt, ObservedAt: c.now(), Provider: ProviderName,
	}, nil
}

func (c *Client) rate(ctx context.Context, currency string, at time.Time) (decimal.Decimal, time.Time, error) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		return decimal.Decimal{}, time.Time{}, err
	}
	requestedInKyiv := at.In(kyiv)
	requestedYear, requestedMonth, requestedDate := requestedInKyiv.Date()
	day := time.Date(requestedYear, requestedMonth, requestedDate, 0, 0, 0, 0, kyiv)
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return decimal.Decimal{}, time.Time{}, err
	}
	query := u.Query()
	query.Set("valcode", currency)
	query.Set("date", day.Format("20060102"))
	query.Set("json", "")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return decimal.Decimal{}, time.Time{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("fetch NBU %s rate: %w", currency, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("fetch NBU %s rate: HTTP %s", currency, resp.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.UseNumber()
	var rows []responseRow
	if err := decoder.Decode(&rows); err != nil {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("decode NBU %s rate: %w", currency, err)
	}
	if len(rows) != 1 || rows[0].Currency != currency {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("NBU returned no unambiguous %s rate for %s", currency, day.Format("2006-01-02"))
	}
	rate, err := decimal.NewFromString(rows[0].Rate.String())
	if err != nil || !rate.IsPositive() {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("invalid NBU %s rate %q", currency, rows[0].Rate)
	}
	effectiveAt, err := time.ParseInLocation("02.01.2006", rows[0].ExchangeDate, kyiv)
	if err != nil {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("parse NBU exchange date: %w", err)
	}
	if effectiveAt.After(day) {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("NBU returned future rate date %s", rows[0].ExchangeDate)
	}
	if effectiveAt.Before(day.AddDate(0, 0, -7)) {
		return decimal.Decimal{}, time.Time{}, fmt.Errorf("NBU returned a rate older than 7 days: %s", rows[0].ExchangeDate)
	}
	effectiveYear, effectiveMonth, effectiveDate := effectiveAt.Date()
	return rate, time.Date(effectiveYear, effectiveMonth, effectiveDate, 0, 0, 0, 0, time.UTC), nil
}
