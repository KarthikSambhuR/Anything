package core

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	WeightVector  = 1.0 // Semantic meaning importance
	WeightKeyword = 1.2 // Exact word match importance
)

func HybridSearch(rawQuery string) ([]SearchResult, error) {
	// 1. NLP Date Parsing
	cleanQuery, minTime, maxTime := ParseDateQuery(rawQuery)

	var wg sync.WaitGroup

	// Declare variables before goroutines
	var vectorResults []SearchResult
	var keywordResults []SearchResult
	var errVector, errKeyword error

	// 2. Parallel Search
	wg.Add(2) // Add both at once for clarity

	go func() {
		defer wg.Done()
		if IsAIReady {
			vectorResults, errVector = SemanticSearch(cleanQuery, minTime, maxTime)
		}
	}()

	go func() {
		defer wg.Done()
		keywordResults, errKeyword = SearchFiles(cleanQuery, minTime, maxTime)
	}()

	wg.Wait()

	// Return early if both searches failed
	if errVector != nil && errKeyword != nil {
		return nil, errKeyword // Or return a combined error
	}

	// 3. Load Usage Stats (Fast RAM lookup)
	usageMap := GetUsageMap()

	// 4. Merge & Rank
	type MergedResult struct {
		Result     SearchResult
		FinalScore float32
	}
	scoreMap := make(map[string]MergedResult)

	// Helper to apply boosts
	calculateBoost := func(path string, baseScore float32) float32 {
		score := baseScore

		// A. Frequency Boost
		if count, ok := usageMap[path]; ok {
			score = score * (1.0 + (float32(count) * 0.5))
		}

		// B. App Type Boost
		lower := strings.ToLower(path)
		if strings.HasSuffix(lower, ".lnk") || strings.HasSuffix(lower, ".exe") {
			score *= 2.0
		}

		// C. Exact Name Match Boost
		name := filepath.Base(lower)
		if strings.HasPrefix(name, strings.ToLower(cleanQuery)) {
			score *= 1.5
		}

		return score
	}

	// Process Keyword Results
	for _, res := range keywordResults {
		scoreMap[res.Path] = MergedResult{
			Result:     res,
			FinalScore: calculateBoost(res.Path, res.Score*WeightKeyword),
		}
	}

	// Merge Vector Results
	for _, res := range vectorResults {
		if existing, found := scoreMap[res.Path]; found {
			// Found in both: "Perfect Match" bonus
			newScore := calculateBoost(res.Path, res.Score*WeightVector)
			existing.FinalScore += newScore + 0.5

			if len(res.Snippet) > len(existing.Result.Snippet) {
				existing.Result.Snippet = res.Snippet
			}
			scoreMap[res.Path] = existing
		} else {
			scoreMap[res.Path] = MergedResult{
				Result:     res,
				FinalScore: calculateBoost(res.Path, res.Score*WeightVector),
			}
		}
	}

	// Sort by FinalScore (descending)
	finalResults := make([]MergedResult, 0, len(scoreMap))
	for _, v := range scoreMap {
		finalResults = append(finalResults, v)
	}

	sort.Slice(finalResults, func(i, j int) bool {
		return finalResults[i].FinalScore > finalResults[j].FinalScore
	})

	// Return top 15 results
	output := make([]SearchResult, 0, 15)
	for i, mr := range finalResults {
		if i >= 15 {
			break
		}
		output = append(output, mr.Result)
	}

	return output, nil
}
