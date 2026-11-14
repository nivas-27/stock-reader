package twelvedata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv(apiKeyEnv, "secret-key")
	t.Setenv(baseURLEnv, srv.URL)
	c := New()
	c.limiter = NewLimiter(1000, 0, time.Second)
	return c
}

func TestTimeSeriesReturnsOldestFirstAndKeepsKeyOutOfURL(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "apikey secret-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		if strings.Contains(r.URL.RawQuery, "secret-key") {
			t.Error("api key leaked into the URL")
		}
		if r.URL.Query().Get("exchange") != "NASDAQ" {
			t.Errorf("exchange = %q", r.URL.Query().Get("exchange"))
		}
		w.Write([]byte(`{"status":"ok","values":[{"close":"3"},{"close":"2"},{"close":"1"}]}`))
	})

	got, err := c.TimeSeries(context.Background(), "AAPL", "NASDAQ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("got %v", got)
	}
}

func TestSymbolSearchMapsFields(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok","data":[{"symbol":"ZM","instrument_name":"Zoom","exchange":"NASDAQ","instrument_type":"Common Stock","country":"United States","currency":"USD"}]}`))
	})

	got, err := c.SymbolSearch(context.Background(), "zoom", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Symbol != "ZM" || got[0].Name != "Zoom" || got[0].Exchange != "NASDAQ" {
		t.Fatalf("got %+v", got)
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"unknown symbol", 200, `{"code":400,"message":"symbol not found","status":"error"}`, func(e error) bool {
			var a *APIError
			return errors.As(e, &a) && a.Message == "symbol not found"
		}},
		{"minute limit", 429, `{"code":429,"message":"run out of API credits for the current minute","status":"error"}`, func(e error) bool {
			var r *RateLimitError
			return errors.As(e, &r) && !r.Daily
		}},
		{"daily limit", 429, `{"code":429,"message":"run out of API credits for the day","status":"error"}`, func(e error) bool {
			var r *RateLimitError
			return errors.As(e, &r) && r.Daily
		}},
		{"bad key", 401, `{"code":401,"message":"apikey invalid","status":"error"}`, func(e error) bool {
			return errors.Is(e, ErrBadKey)
		}},
		{"server error", 502, `{"code":502,"message":"bad gateway","status":"error"}`, func(e error) bool {
			return errors.Is(e, ErrTransient)
		}},
		{"garbage", 200, `<html>`, func(e error) bool {
			return errors.Is(e, ErrTransient)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			})
			_, err := c.TimeSeries(context.Background(), "X", "")
			if err == nil || !tc.check(err) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNoKey(t *testing.T) {
	t.Setenv(apiKeyEnv, "")
	if _, err := New().SymbolSearch(context.Background(), "x", true); !errors.Is(err, ErrNoKey) {
		t.Fatalf("got %v", err)
	}
}
