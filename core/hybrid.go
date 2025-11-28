package core

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// The "Feeling Good" Config
const (
	WeightVector  = 1.0 // Semantic meaning importance
	WeightKeyword = 1.2 // Exact word match importance (Keep this high for "launcher" feel)
)

func HybridSearch(rawQuery string) ([]SearchResult, error) {
	// 1. PARSE DATE (NLP)
	// "report last week" -> query="report", min=..., max=...
	cleanQuery, minTime, maxTime := ParseDateQuery(rawQuery)

	// If date was found, print it for debug
	if minTime > 0 {
		fmt.Printf("[NLP] Filtering files modified after: %s\n", time.Unix(minTime, 0).Format(time.RFC822))
	}

	var wg sync.WaitGroup
	var vectorResults []SearchResult
	var keywordResults []SearchResult

	// 2. Vector Search (Pass times)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if IsAIReady {
			vectorResults, _ = SemanticSearch(cleanQuery, minTime, maxTime)
		}
	}()

	// 3. Keyword Search (Pass times)
	wg.Add(1)
	go func() {
		defer wg.Done()
		keywordResults, _ = SearchFiles(cleanQuery, minTime, maxTime)
	}()

	wg.Wait()

	// ... (Rest of Hybrid Logic remains exactly the same) ...
	// Just copy the merging logic from previous hybrid.go code here

	// --- MERGE LOGIC START ---
	type MergedResult struct {
		Result     SearchResult
		FinalScore float32
	}
	scoreMap := make(map[string]MergedResult)

	for _, res := range keywordResults {
		scoreMap[res.Path] = MergedResult{Result: res, FinalScore: res.Score * WeightKeyword}
	}
	for _, res := range vectorResults {
		if existing, found := scoreMap[res.Path]; found {
			existing.FinalScore += (res.Score * WeightVector) + 0.5
			if len(res.Snippet) > len(existing.Result.Snippet) {
				existing.Result.Snippet = res.Snippet
			}
			scoreMap[res.Path] = existing
		} else {
			scoreMap[res.Path] = MergedResult{Result: res, FinalScore: res.Score * WeightVector}
		}
	}

	var finalResults []MergedResult
	for _, v := range scoreMap {
		finalResults = append(finalResults, v)
	}
	sort.Slice(finalResults, func(i, j int) bool { return finalResults[i].FinalScore > finalResults[j].FinalScore })

	var output []SearchResult
	for i, mr := range finalResults {
		if i >= 15 {
			break
		}
		output = append(output, mr.Result)
	}
	return output, nil
}
