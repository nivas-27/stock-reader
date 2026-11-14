package twelvedata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"backend/models"
)

const (
	defaultBaseURL = "https://api.twelvedata.com"
	apiKeyEnv      = "TWELVE_DATA_API_KEY"
	baseURLEnv     = "TWELVE_DATA_BASE_URL"   // lets tests point at a fake server
	perMinuteEnv   = "TWELVE_DATA_PER_MINUTE" // raise on a paid plan

	// Free plan: 8 requests a minute. 2 are kept free for interactive calls.
	defaultPerMinute = 8
	reservedForUsers = 2
	window           = 61 * time.Second

	historyDays  = 40 // enough closes for a 20-day average and its 5-day slope
	maxBodyBytes = 4 << 20
)

var (
	ErrNoKey     = errors.New("TWELVE_DATA_API_KEY is not set")
	ErrBadKey    = errors.New("Twelve Data rejected the API key")
	ErrTransient = errors.New("temporary Twelve Data failure")
)

// RateLimitError means a minute or daily limit was hit.
type RateLimitError struct {
	Daily   bool
	Message string
}

func (e *RateLimitError) Error() string { return "twelve data rate limit: " + e.Message }

// APIError is a definite answer for this request (unknown symbol, plan has no access...).
// Retrying the same request will not help.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string { return e.Message }

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	limiter *Limiter
}

func New() *Client {
	base := os.Getenv(baseURLEnv)
	if base == "" {
		base = defaultBaseURL
	}
	perMinute := defaultPerMinute
	if n, err := strconv.Atoi(os.Getenv(perMinuteEnv)); err == nil && n > 0 {
		perMinute = n
	}
	reserve := reservedForUsers
	if perMinute <= reserve {
		reserve = 0
	}

	return &Client{
		baseURL: strings.TrimRight(base, "/"),
		apiKey:  os.Getenv(apiKeyEnv),
		http:    &http.Client{Timeout: 20 * time.Second},
		limiter: NewLimiter(perMinute, reserve, window),
	}
}

func (c *Client) HasKey() bool { return c.apiKey != "" }

type envelope struct {
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// get sends one rate-limited request and decodes the body into out.
// The key travels in a header so it can never show up in a URL-bearing error.
func (c *Client) get(ctx context.Context, path string, q url.Values, interactive bool, out any) error {
	if !c.HasKey() {
		return ErrNoKey
	}
	if err := c.limiter.Wait(ctx, interactive); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("%w: request build failed", ErrTransient)
	}
	req.Header.Set("Authorization", "apikey "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: request failed", ErrTransient)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: response read failed", ErrTransient)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: unreadable response (HTTP %d)", ErrTransient, resp.StatusCode)
	}

	code := env.Code
	if code == 0 {
		code = resp.StatusCode
	}
	if env.Status == "error" || resp.StatusCode >= 400 {
		return classify(code, env.Message)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: unexpected response shape", ErrTransient)
	}
	return nil
}

func classify(code int, message string) error {
	switch {
	case code == http.StatusTooManyRequests:
		return &RateLimitError{Daily: strings.Contains(strings.ToLower(message), "day"), Message: message}
	case code == http.StatusUnauthorized:
		return ErrBadKey
	case code >= 500:
		return fmt.Errorf("%w: HTTP %d", ErrTransient, code)
	default:
		if message == "" {
			message = "Twelve Data returned an error (" + strconv.Itoa(code) + ")"
		}
		return &APIError{Code: code, Message: message}
	}
}

// TimeSeries returns daily closes, oldest first (1 credit). Background priority.
func (c *Client) TimeSeries(ctx context.Context, symbol, exchange string) ([]float64, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("interval", "1day")
	q.Set("outputsize", strconv.Itoa(historyDays))
	if exchange != "" {
		q.Set("exchange", exchange)
	}

	var res struct {
		Values []struct {
			Close string `json:"close"`
		} `json:"values"`
	}
	if err := c.get(ctx, "/time_series", q, false, &res); err != nil {
		return nil, err
	}

	closes := make([]float64, 0, len(res.Values))
	for i := len(res.Values) - 1; i >= 0; i-- { // API returns newest first
		if v, err := strconv.ParseFloat(res.Values[i].Close, 64); err == nil {
			closes = append(closes, v)
		}
	}
	return closes, nil
}

// SymbolSearch finds instruments by symbol or name (1 credit).
func (c *Client) SymbolSearch(ctx context.Context, query string, interactive bool) ([]models.SymbolMatch, error) {
	q := url.Values{}
	q.Set("symbol", query)
	q.Set("outputsize", "15")

	var res struct {
		Data []struct {
			Symbol   string `json:"symbol"`
			Name     string `json:"instrument_name"`
			Exchange string `json:"exchange"`
			Type     string `json:"instrument_type"`
			Country  string `json:"country"`
			Currency string `json:"currency"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/symbol_search", q, interactive, &res); err != nil {
		return nil, err
	}

	out := make([]models.SymbolMatch, 0, len(res.Data))
	for _, d := range res.Data {
		out = append(out, models.SymbolMatch{
			Symbol: d.Symbol, Name: d.Name, Exchange: d.Exchange,
			Type: d.Type, Country: d.Country, Currency: d.Currency,
		})
	}
	return out, nil
}
