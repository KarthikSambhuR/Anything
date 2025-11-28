package core

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	WeightVector  = 1.0 // Semantic meaning importance
	WeightKeyword = 1.2 // Exact word match importance
)

func HybridSearch(rawQuery string) ([]SearchResult, error) {
	// 1. NLP Date Parsing
	cleanQuery, minTime, maxTime := ParseDateQuery(rawQuery)

	if minTime > 0 {
		fmt.Printf("[NLP] Filtering files modified after: %s\n", time.Unix(minTime, 0).Format(time.RFC822))
	}

	var wg sync.WaitGroup
	var vectorResults []SearchResult
	var keywordResults []SearchResult
	var errVector, errKeyword error

	// 2. Vector Search (Semantic)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if IsAIReady {
			vectorResults, errVector = SemanticSearch(cleanQuery, minTime, maxTime)
		}
	}()

	// 3. Keyword Search (FTS5)
	wg.Add(1)
	go func() {
		defer wg.Done()
		keywordResults, errKeyword = SearchFiles(cleanQuery, minTime, maxTime)
	}()

	wg.Wait()

	if errVector != nil {
		fmt.Printf("Vector Error: %v\n", errVector)
	}
	if errKeyword != nil {
		fmt.Printf("Keyword Error: %v\n", errKeyword)
	}

	// --- MERGE & RANKING LOGIC ---
	type MergedResult struct {
		Result     SearchResult
		FinalScore float32
	}
	scoreMap := make(map[string]MergedResult)

	// Process Keyword Results
	for _, res := range keywordResults {
		scoreMap[res.Path] = MergedResult{
			Result:     res,
			FinalScore: res.Score * WeightKeyword,
		}
	}

	// Merge Vector Results
	for _, res := range vectorResults {
		if existing, found := scoreMap[res.Path]; found {
			// Found in both: "Perfect Match" bonus
			existing.FinalScore += (res.Score * WeightVector) + 0.5

			// Prefer the snippet from Vector search (usually better context)
			if len(res.Snippet) > len(existing.Result.Snippet) {
				existing.Result.Snippet = res.Snippet
			}
			scoreMap[res.Path] = existing
		} else {
			scoreMap[res.Path] = MergedResult{
				Result:     res,
				FinalScore: res.Score * WeightVector,
			}
		}
	}

	// --- APP BOOSTING ---
	// Prioritize executables and shortcuts to act as a launcher
	for path, item := range scoreMap {
		lowerPath := strings.ToLower(path)

		if strings.HasSuffix(lowerPath, ".lnk") || strings.HasSuffix(lowerPath, ".exe") {
			// Primary Boost for Apps
			item.FinalScore *= 10.0

			// Secondary Boost: Prefix match (e.g. Query "Chr" matches "Chrome")
			name := filepath.Base(lowerPath)
			if strings.HasPrefix(name, strings.ToLower(cleanQuery)) {
				item.FinalScore *= 2.0
			}

			scoreMap[path] = item
		}
	}

	// 4. Sort and Limit
	var finalResults []MergedResult
	for _, v := range scoreMap {
		finalResults = append(finalResults, v)
	}

	sort.Slice(finalResults, func(i, j int) bool {
		return finalResults[i].FinalScore > finalResults[j].FinalScore
	})

	var output []SearchResult
	for i, mr := range finalResults {
		if i >= 15 {
			break
		}
		output = append(output, mr.Result)
	}

	return output, nil
}
