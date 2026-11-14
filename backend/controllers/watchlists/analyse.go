package watchlists

import (
	"errors"
	"time"

	"backend/models"
)

const (
	smaPeriod   = 20
	smaLookback = 5
	trendSlope  = 0.5 // % move of the average over the lookback that counts as a trend
	sparkPoints = 30
)

// buildQuote turns oldest-first daily closes into the day's price, change and trend.
func buildQuote(key, day string, closes []float64) (models.Quote, error) {
	n := len(closes)
	if n < 2 {
		return models.Quote{}, errors.New("not enough price history")
	}

	last, prev := closes[n-1], closes[n-2]
	spark := closes
	if n > sparkPoints {
		spark = closes[n-sparkPoints:]
	}

	return models.Quote{
		Key:       key,
		Day:       day,
		Price:     last,
		Change:    last - prev,
		ChangePct: (last/prev - 1) * 100,
		Trend:     trendOf(closes),
		Spark:     spark,
		FetchedAt: time.Now(),
	}, nil
}

// Above a rising 20-day average is up, below a falling one is down, anything else sideways.
func trendOf(closes []float64) string {
	n := len(closes)
	if n < smaPeriod+smaLookback {
		return "Sideways"
	}

	now := smaAt(closes, n-1)
	before := smaAt(closes, n-1-smaLookback)
	slope := (now/before - 1) * 100
	last := closes[n-1]

	if last > now && slope > trendSlope {
		return "Uptrend"
	}
	if last < now && slope < -trendSlope {
		return "Downtrend"
	}
	return "Sideways"
}

func smaAt(closes []float64, end int) float64 {
	sum := 0.0
	for _, v := range closes[end-smaPeriod+1 : end+1] {
		sum += v
	}
	return sum / smaPeriod
}
