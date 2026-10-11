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
	cleanQuery := strings.TrimSpace(rawQuery)

	var wg sync.WaitGroup

	// Declare variables before goroutines
	var vectorResults []SearchResult
	var keywordResults []SearchResult
	var errVector, errKeyword error

	// 2. Parallel Search
	wg.Add(2) // Add both at once for clarity

	go func() {
		defer wg.Done()
		if IsAIReady && CurrentSettings.AIEnabled {
			vectorResults, errVector = SemanticSearch(cleanQuery, 0, 0)
		}
	}()

	go func() {
		defer wg.Done()
		keywordResults, errKeyword = SearchFiles(cleanQuery, 0, 0)
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
		queryLower := strings.ToLower(cleanQuery)
		
		// If exact match or exact prefix match without extension, boost to the top
		nameWithoutExt := strings.TrimSuffix(name, filepath.Ext(name))
		if name == queryLower || nameWithoutExt == queryLower {
			score += 10000.0
		} else if strings.HasPrefix(name, queryLower) || strings.HasPrefix(nameWithoutExt, queryLower) {
			score += 5000.0
		}

		// D. Priority Directory Boost
		for _, prioDir := range CurrentSettings.PriorityDirectories {
			if strings.HasPrefix(lower, strings.ToLower(prioDir)) {
				score *= 1.5
			}
		}

		return score
	}

	// Process Installed Apps (High Priority)
	if cleanQuery != "" {
		appRows, err := DB.Query("SELECT path, display_name, COALESCE(icon_data, ''), COALESCE(publisher, '') FROM installed_apps WHERE display_name LIKE ? OR name LIKE ? LIMIT 5", "%"+cleanQuery+"%", "%"+cleanQuery+"%")
		if err == nil {
			for appRows.Next() {
				var r SearchResult
				var pub string
				if err := appRows.Scan(&r.Path, &r.Snippet, &r.IconData, &pub); err == nil {
					scoreMap[r.Path] = MergedResult{
						Result:     r,
						FinalScore: 25000.0,
					}
				}
			}
			appRows.Close()
		}
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
