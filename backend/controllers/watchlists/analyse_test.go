package watchlists

import "testing"

func series(n int, start, step float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = start + step*float64(i)
	}
	return out
}

func TestTrendOf(t *testing.T) {
	cases := []struct {
		name   string
		closes []float64
		want   string
	}{
		{"rising", series(40, 100, 1), "Uptrend"},
		{"falling", series(40, 140, -1), "Downtrend"},
		{"flat", series(40, 100, 0), "Sideways"},
		{"too short", series(10, 100, 5), "Sideways"},
	}
	for _, c := range cases {
		if got := trendOf(c.closes); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestBuildQuote(t *testing.T) {
	q, err := buildQuote("AAPL|NASDAQ", "2026-11-12", series(40, 100, 1))
	if err != nil {
		t.Fatal(err)
	}
	if q.Price != 139 || q.Change != 1 {
		t.Errorf("price/change = %v/%v", q.Price, q.Change)
	}
	if len(q.Spark) != sparkPoints {
		t.Errorf("spark has %d points, want %d", len(q.Spark), sparkPoints)
	}
	if q.ChangePct < 0.7 || q.ChangePct > 0.8 {
		t.Errorf("changePct = %v", q.ChangePct)
	}

	if _, err := buildQuote("X|", "d", []float64{1}); err == nil {
		t.Error("expected error for a single close")
	}
}
