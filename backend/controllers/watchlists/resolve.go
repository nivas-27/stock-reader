package watchlists

import (
	"context"
	"fmt"
	"strings"

	"backend/models"
)

const (
	scoreSymbolExact = 100
	scoreNameExact   = 80
	scoreNameHas     = 40
	minScore         = scoreNameHas // weaker matches are reported as unresolved, not guessed
	confidentScore   = scoreSymbolExact
)

var (
	wantedTypes    = map[string]bool{"common stock": true, "etf": true, "physical currency": true, "digital currency": true}
	primaryVenues  = map[string]bool{"nasdaq": true, "nyse": true, "nyse arca": true}
	entrySeparator = strings.NewReplacer("\n", ",", ";", ",")
)

// parseEntries splits "AAPL, Tesla; NVDA - Nvidia" into trimmed, de-duplicated entries.
func parseEntries(raw string) ([]string, error) {
	var out []string
	seen := map[string]bool{}

	for _, part := range strings.Split(entrySeparator.Replace(raw), ",") {
		e := strings.TrimSpace(part)
		if e == "" || seen[strings.ToLower(e)] {
			continue
		}
		if len([]rune(e)) > maxEntryLen {
			return nil, fmt.Errorf("entry %q... is longer than %d characters", string([]rune(e)[:20]), maxEntryLen)
		}
		seen[strings.ToLower(e)] = true
		out = append(out, e)
	}

	if len(out) > maxEntriesBatch {
		return nil, fmt.Errorf("at most %d entries at a time", maxEntriesBatch)
	}
	return out, nil
}

// entryTerms reads an entry as a symbol, a name, or both ("NVDA - Nvidia" or "NVDA | Nvidia").
func entryTerms(entry string) []string {
	var parts []string
	switch {
	case strings.Contains(entry, "|"):
		parts = strings.SplitN(entry, "|", 2)
	case strings.Contains(entry, " - "):
		parts = strings.SplitN(entry, " - ", 2)
	default:
		parts = []string{entry}
	}

	terms := make([]string, 0, 2)
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			terms = append(terms, p)
		}
	}
	return terms
}

// pickBest scores each match against the terms and returns the strongest one.
// A listing on a main US venue beats any other listing that matches at all
// (the free plan only serves US data, so "zoom" should give ZM, not ZOOM in Sweden).
// Ties keep the API's own relevance order.
func pickBest(matches []models.SymbolMatch, terms []string) (models.SymbolMatch, int) {
	var best models.SymbolMatch
	bestScore, bestPrimary := 0, false

	for _, m := range matches {
		score := matchScore(m, terms)
		if score == 0 {
			continue
		}
		primary := primaryVenues[strings.ToLower(m.Exchange)]
		if (primary && !bestPrimary) || (primary == bestPrimary && score > bestScore) {
			best, bestScore, bestPrimary = m, score, primary
		}
	}
	return best, bestScore
}

func matchScore(m models.SymbolMatch, terms []string) int {
	name := strings.ToLower(m.Name)
	score := 0

	for _, t := range terms {
		t = strings.ToLower(t)
		switch {
		case strings.EqualFold(m.Symbol, t):
			score += scoreSymbolExact
		case name == t:
			score += scoreNameExact
		case strings.Contains(name, t):
			score += scoreNameHas
		}
	}

	if score == 0 {
		return 0 // unrelated; the small bonuses below must not rescue it
	}
	if wantedTypes[strings.ToLower(m.Type)] {
		score += 10
	}
	return score
}

func mergeMatches(have, more []models.SymbolMatch) []models.SymbolMatch {
	seen := map[string]bool{}
	for _, m := range have {
		seen[itemKey(m.Symbol, m.Exchange)] = true
	}
	for _, m := range more {
		if k := itemKey(m.Symbol, m.Exchange); !seen[k] {
			seen[k] = true
			have = append(have, m)
		}
	}
	return have
}

func toItem(m models.SymbolMatch) models.ListItem {
	return models.ListItem{Symbol: m.Symbol, Name: m.Name, Exchange: m.Exchange, Type: m.Type}
}

// resolveEntry looks the entry up through the cached search. It only spends a
// second search when the first term did not settle the answer.
func (ct *Controller) resolveEntry(ctx context.Context, entry string) (models.ListItem, bool, error) {
	terms := entryTerms(entry)
	var all []models.SymbolMatch

	for _, t := range terms {
		found, err := ct.searchCached(ctx, t, false)
		if err != nil {
			return models.ListItem{}, false, err
		}
		all = mergeMatches(all, found)
		if best, score := pickBest(all, terms); score >= confidentScore {
			return toItem(best), true, nil
		}
	}

	best, score := pickBest(all, terms)
	if score < minScore {
		return models.ListItem{}, false, nil
	}
	return toItem(best), true, nil
}
