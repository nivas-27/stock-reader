package kitehistorical

import (
	env "backend/configs/env"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultPage  = 1
	defaultLimit = 10
	maxLimit     = 100
	dateLayout   = "2006-01-02"
	stockFile    = "Nifty_Smallcap_250_Stock_Details.js"
)

type stockMeta struct {
	Token    string `json:"Token"`
	Name     string `json:"Name"`
	Category string `json:"category"`
}

type historicalItem struct {
	Token    string          `json:"token"`
	Name     string          `json:"name"`
	Category string          `json:"category"`
	Candles  [][]interface{} `json:"candles"`
	Error    string          `json:"error,omitempty"`
}

type historicalResponse struct {
	Page        int              `json:"page"`
	Limit       int              `json:"limit"`
	TotalStocks int              `json:"totalStocks"`
	TotalPages  int              `json:"totalPages"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	Data        []historicalItem `json:"data"`
}

type kiteHistoricalAPIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Candles [][]interface{} `json:"candles"`
	} `json:"data"`
}

var (
	stocksOnce sync.Once
	stocks     []stockMeta
	stocksErr  error
)

func GetHistoricalData(c *gin.Context) {
	page, err := parsePositiveInt(c.Query("page"), defaultPage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page must be a positive integer"})
		return
	}

	limit, err := parsePositiveInt(c.Query("limit"), defaultLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
		return
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	fromDate, toDate, err := resolveDateRange(c.Query("from"), c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stockList, err := loadStockList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalStocks := len(stockList)
	totalPages := 0
	if totalStocks > 0 {
		totalPages = (totalStocks + limit - 1) / limit
	}

	pagedStocks := paginateStocks(stockList, page, limit)

	apiKey := env.GetEnv("KITE_API_KEY")
	accessToken := env.GetEnv("KITE_ACCESS_TOKEN")
	if apiKey == "" || accessToken == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "KITE_API_KEY or KITE_ACCESS_TOKEN is missing"})
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	items := make([]historicalItem, 0, len(pagedStocks))

	for _, stock := range pagedStocks {
		candles, fetchErr := fetchHistoricalForToken(client, stock.Token, fromDate, toDate, apiKey, accessToken)
		item := historicalItem{
			Token:    stock.Token,
			Name:     stock.Name,
			Category: stock.Category,
			Candles:  candles,
		}
		if fetchErr != nil {
			item.Candles = [][]interface{}{}
			item.Error = fetchErr.Error()
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, historicalResponse{
		Page:        page,
		Limit:       limit,
		TotalStocks: totalStocks,
		TotalPages:  totalPages,
		From:        fromDate.Format(dateLayout),
		To:          toDate.Format(dateLayout),
		Data:        items,
	})
}

func parsePositiveInt(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 0, errors.New("invalid positive integer")
	}
	return v, nil
}

func resolveDateRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	toDate := time.Now()
	if strings.TrimSpace(toRaw) != "" {
		parsed, err := time.Parse(dateLayout, toRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("to must be in YYYY-MM-DD format")
		}
		toDate = parsed
	}

	fromDate := toDate.AddDate(0, 0, -30)
	if strings.TrimSpace(fromRaw) != "" {
		parsed, err := time.Parse(dateLayout, fromRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("from must be in YYYY-MM-DD format")
		}
		fromDate = parsed
	}

	if fromDate.After(toDate) {
		return time.Time{}, time.Time{}, fmt.Errorf("from date cannot be greater than to date")
	}

	return fromDate, toDate, nil
}

func paginateStocks(all []stockMeta, page, limit int) []stockMeta {
	start := (page - 1) * limit
	if start >= len(all) {
		return []stockMeta{}
	}
	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end]
}

func loadStockList() ([]stockMeta, error) {
	stocksOnce.Do(func() {
		path, err := findStockFilePath()
		if err != nil {
			stocksErr = err
			return
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			stocksErr = fmt.Errorf("failed to read stock metadata: %w", err)
			return
		}

		if err := json.Unmarshal(raw, &stocks); err != nil {
			stocksErr = fmt.Errorf("failed to parse stock metadata: %w", err)
			return
		}
	})

	if stocksErr != nil {
		return nil, stocksErr
	}
	return stocks, nil
}

func findStockFilePath() (string, error) {
	candidates := []string{
		filepath.Join("stock-data", stockFile),
		filepath.Join("backend", "stock-data", stockFile),
		filepath.Join("..", "backend", "stock-data", stockFile),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("stock metadata file not found")
}

func fetchHistoricalForToken(client *http.Client, token string, fromDate, toDate time.Time, apiKey, accessToken string) ([][]interface{}, error) {
	endpoint := fmt.Sprintf(
		"https://api.kite.trade/instruments/historical/%s/day?from=%s&to=%s",
		url.PathEscape(token),
		fromDate.Format(dateLayout),
		toDate.Format(dateLayout),
	)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("request build failed: %w", err)
	}
	req.Header.Set("X-Kite-Version", "3")
	req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", apiKey, accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("response read failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kite api returned status %d: %s", resp.StatusCode, previewBody(body))
	}

	var parsed kiteHistoricalAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("response parse failed: %w", err)
	}

	if strings.ToLower(parsed.Status) != "success" {
		if parsed.Message == "" {
			return nil, errors.New("kite api returned non-success response")
		}
		return nil, errors.New(parsed.Message)
	}

	if parsed.Data.Candles == nil {
		return [][]interface{}{}, nil
	}

	return parsed.Data.Candles, nil
}

func previewBody(body []byte) string {
	clean := strings.TrimSpace(string(body))
	if len(clean) > 200 {
		return clean[:200] + "..."
	}
	if clean == "" {
		return "empty body"
	}
	return clean
}
