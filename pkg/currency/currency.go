package currency

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client fetches and caches exchange rates.
type Client interface {
	// Rate returns the exchange rate from the given currency to USD.
	// e.g., if currency is EUR and 1 EUR = 1.10 USD, returns 1.10.
	Rate(currency string) (float64, error)

	// Convert converts an amount in cents from one currency to USD cents.
	Convert(amountCents int64, fromCurrency string) (usdCents int64, rate float64, err error)

	// ConvertFromUSD converts USD cents to another currency's cents using the current rate.
	ConvertFromUSD(usdCents int64, toCurrency string) (int64, error)
}

// ExchangeRateClient is the live implementation of Client.
type ExchangeRateClient struct {
	baseURL string
	apiKey  string

	mu        sync.RWMutex
	rates     map[string]float64
	fetchedAt time.Time
	ttl       time.Duration
}

// NewClient creates a new exchange rate client.
// It uses the exchangerate-api.com free API.
func NewClient(apiKey string) *ExchangeRateClient {
	return &ExchangeRateClient{
		baseURL: "https://v6.exchangerate-api.com/v6",
		apiKey:  apiKey,
		rates:   make(map[string]float64),
		ttl:     1 * time.Hour,
	}
}

type apiResponse struct {
	Result          string             `json:"result"`
	ConversionRates map[string]float64 `json:"conversion_rates"`
}

func (c *ExchangeRateClient) fetchRates() error {
	resp, err := http.Get(fmt.Sprintf("%s/%s/latest/USD", c.baseURL, c.apiKey))
	if err != nil {
		return fmt.Errorf("fetch exchange rates: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("exchange rate API returned status %d", resp.StatusCode)
	}

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode exchange rates: %w", err)
	}
	if data.Result != "success" {
		return fmt.Errorf("exchange rate API error: %s", data.Result)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.rates = data.ConversionRates
	c.fetchedAt = time.Now()
	return nil
}

func (c *ExchangeRateClient) ensureFresh() error {
	c.mu.RLock()
	fresh := time.Since(c.fetchedAt) < c.ttl && len(c.rates) > 0
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	return c.fetchRates()
}

func (c *ExchangeRateClient) Rate(currency string) (float64, error) {
	currency = strings.ToUpper(currency)
	if currency == "USD" {
		return 1.0, nil
	}
	if err := c.ensureFresh(); err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	rate, ok := c.rates[currency]
	if !ok {
		return 0, fmt.Errorf("unknown currency: %s", currency)
	}
	// rates map has USD as base (1 USD = X currency).
	// We want: 1 <currency> = ? USD, so invert.
	return 1.0 / rate, nil
}

func (c *ExchangeRateClient) Convert(amountCents int64, fromCurrency string) (int64, float64, error) {
	fromCurrency = strings.ToUpper(fromCurrency)
	if fromCurrency == "USD" {
		return amountCents, 1.0, nil
	}
	rate, err := c.Rate(fromCurrency)
	if err != nil {
		return 0, 0, err
	}
	usdCents := int64(math.Round(float64(amountCents) * rate))
	return usdCents, rate, nil
}

func (c *ExchangeRateClient) ConvertFromUSD(usdCents int64, toCurrency string) (int64, error) {
	toCurrency = strings.ToUpper(toCurrency)
	if toCurrency == "USD" {
		return usdCents, nil
	}
	if err := c.ensureFresh(); err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	rate, ok := c.rates[toCurrency]
	if !ok {
		return 0, fmt.Errorf("unknown currency: %s", toCurrency)
	}
	// 1 USD = rate <toCurrency>, so multiply.
	return int64(math.Round(float64(usdCents) * rate)), nil
}

// IsValidCurrency checks if a currency code looks valid (3 uppercase letters).
func IsValidCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
