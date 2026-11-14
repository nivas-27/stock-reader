package watchlists

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"backend/utils/twelvedata"

	"github.com/gin-gonic/gin"
)

// Search finds instruments by symbol or name, including ones in no list yet.
func (ct *Controller) Search(c *gin.Context) {
	type result struct {
		Symbol   string `json:"symbol"`
		Name     string `json:"name"`
		Exchange string `json:"exchange"`
		Type     string `json:"type"`
		Country  string `json:"country"`
		Currency string `json:"currency"`
	}

	q := strings.TrimSpace(c.Query("q"))
	if n := len([]rune(q)); n < 2 || n > 64 {
		fail(c, http.StatusBadRequest, "q must be 2-64 characters")
		return
	}

	matches, err := ct.searchCached(c.Request.Context(), q, true)
	if err != nil {
		var limit *twelvedata.RateLimitError
		var api *twelvedata.APIError
		switch {
		case errors.Is(err, twelvedata.ErrNoKey):
			fail(c, http.StatusServiceUnavailable, "TWELVE_DATA_API_KEY is not set on the server")
		case errors.Is(err, context.Canceled):
			c.Status(http.StatusRequestTimeout)
		case errors.As(err, &limit):
			fail(c, http.StatusTooManyRequests, "Twelve Data limit reached, try again shortly")
		case errors.As(err, &api):
			fail(c, http.StatusBadGateway, api.Message)
		default:
			fail(c, http.StatusBadGateway, "search is unavailable right now")
		}
		return
	}

	out := make([]result, 0, len(matches))
	for _, m := range matches {
		out = append(out, result{m.Symbol, m.Name, m.Exchange, m.Type, m.Country, m.Currency})
	}
	c.JSON(http.StatusOK, gin.H{"results": out})
}
