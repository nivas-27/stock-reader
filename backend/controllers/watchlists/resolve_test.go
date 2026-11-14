package watchlists

import (
	"reflect"
	"strings"
	"testing"

	"backend/models"
)

func TestParseEntries(t *testing.T) {
	got, err := parseEntries(" AAPL, tesla ;NVDA - Nvidia\nzm,, aapl ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"AAPL", "tesla", "NVDA - Nvidia", "zm"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseEntriesLimits(t *testing.T) {
	if _, err := parseEntries(strings.Repeat("x", maxEntryLen+1)); err == nil {
		t.Error("expected error for an over-long entry")
	}

	var many []string
	for i := 0; i <= maxEntriesBatch; i++ {
		many = append(many, "S"+strings.Repeat("a", i))
	}
	if _, err := parseEntries(strings.Join(many, ",")); err == nil {
		t.Error("expected error for too many entries")
	}
}

func TestEntryTerms(t *testing.T) {
	cases := map[string][]string{
		"AAPL":          {"AAPL"},
		"Tesla Inc":     {"Tesla Inc"},
		"NVDA - Nvidia": {"NVDA", "Nvidia"},
		"NVDA | Nvidia": {"NVDA", "Nvidia"},
		"USD/JPY":       {"USD/JPY"},
	}
	for in, want := range cases {
		if got := entryTerms(in); !reflect.DeepEqual(got, want) {
			t.Errorf("entryTerms(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickBest(t *testing.T) {
	matches := []models.SymbolMatch{
		{Symbol: "TSLA", Name: "Tesla Inc", Exchange: "MEX", Type: "Common Stock"},
		{Symbol: "TSLA", Name: "Tesla Inc", Exchange: "NASDAQ", Type: "Common Stock"},
		{Symbol: "TSLY", Name: "YieldMax TSLA Option Income Strategy ETF", Exchange: "NYSE ARCA", Type: "ETF"},
	}

	t.Run("symbol prefers the primary venue", func(t *testing.T) {
		best, score := pickBest(matches, []string{"tsla"})
		if best.Exchange != "NASDAQ" || score < scoreSymbolExact {
			t.Fatalf("got %+v score %d", best, score)
		}
	})

	t.Run("company name works", func(t *testing.T) {
		best, score := pickBest(matches, []string{"Tesla"})
		if best.Symbol != "TSLA" || best.Exchange != "NASDAQ" || score < minScore {
			t.Fatalf("got %+v score %d", best, score)
		}
	})

	t.Run("symbol plus name agrees", func(t *testing.T) {
		best, _ := pickBest(matches, []string{"TSLA", "Tesla"})
		if best.Symbol != "TSLA" {
			t.Fatalf("got %+v", best)
		}
	})

	t.Run("US listing beats an exact symbol on a foreign venue", func(t *testing.T) {
		zoom := []models.SymbolMatch{
			{Symbol: "ZOOM", Name: "Zoomability AB", Exchange: "OMX", Type: "Common Stock"},
			{Symbol: "ZM", Name: "Zoom Communications, Inc. Class A Common Stock", Exchange: "NASDAQ", Type: "Common Stock"},
		}
		if best, _ := pickBest(zoom, []string{"zoom"}); best.Symbol != "ZM" {
			t.Fatalf("got %+v", best)
		}
	})

	t.Run("falls back to the best foreign match when no US one fits", func(t *testing.T) {
		foreign := []models.SymbolMatch{
			{Symbol: "RELIANCE", Name: "Reliance Industries", Exchange: "NSE", Type: "Common Stock"},
			{Symbol: "RELIANCE", Name: "Reliance Industries", Exchange: "BSE", Type: "Common Stock"},
		}
		best, score := pickBest(foreign, []string{"RELIANCE"})
		if best.Exchange != "NSE" || score < minScore {
			t.Fatalf("got %+v score %d", best, score)
		}
	})

	t.Run("unrelated results are not guessed", func(t *testing.T) {
		if _, score := pickBest(matches, []string{"banana"}); score >= minScore {
			t.Fatalf("score %d should be below %d", score, minScore)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		if _, score := pickBest(nil, []string{"x"}); score != 0 {
			t.Fatalf("score %d", score)
		}
	})
}
