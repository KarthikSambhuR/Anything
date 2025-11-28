package core

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"
)

// Data structure for RAM Cache
type CachedVector struct {
	FileID     int
	ChunkIndex int
	Data       []float32
}

// Global Cache
var VectorIndex []CachedVector

// 1. Math: Cosine Similarity
// Since vectors are already normalized (L2=1) in GetEmbedding,
// Cosine Similarity is just the Dot Product.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0.0
	}
	var dot float32 = 0.0
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	return dot
}

// 2. Load DB into RAM (Call this on startup)
func LoadVectorIndex() {
	fmt.Print("Loading Vector Index into RAM... ")
	startTime := time.Now()

	rows, err := DB.Query("SELECT file_id, chunk_index, vector_blob FROM file_vectors")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer rows.Close()

	VectorIndex = []CachedVector{} // Reset

	for rows.Next() {
		var fileID, chunkIdx int
		var blob []byte

		if err := rows.Scan(&fileID, &chunkIdx, &blob); err != nil {
			continue
		}

		// Convert Blob ([]byte) -> Vector ([]float32)
		// We know it's float32 (4 bytes), so len/4
		vecLen := len(blob) / 4
		vec := make([]float32, vecLen)

		for i := 0; i < vecLen; i++ {
			bits := binary.LittleEndian.Uint32(blob[i*4 : (i+1)*4])
			vec[i] = math.Float32frombits(bits)
		}

		VectorIndex = append(VectorIndex, CachedVector{
			FileID:     fileID,
			ChunkIndex: chunkIdx,
			Data:       vec,
		})
	}

	fmt.Printf("Done! Loaded %d vectors in %v\n", len(VectorIndex), time.Since(startTime))
}

// 3. The Search Function
// Returns File Paths sorted by relevance
func SemanticSearch(query string, minTime int64, maxTime int64) ([]SearchResult, error) {
	if !IsAIReady || len(VectorIndex) == 0 {
		return nil, fmt.Errorf("AI not ready or index empty")
	}

	// 1. Load File Metadata (Time) for filtering
	// Optimization: Since VectorIndex is in RAM, we need a fast way to check dates without querying SQL for every vector.
	// Let's assume we filter AFTER finding matches for simplicity in v0.3.

	queryVec, err := GetEmbedding(query)
	if err != nil {
		return nil, err
	}

	type Match struct {
		FileID int
		Score  float32
	}
	fileScores := make(map[int]float32)
	threshold := float32(0.35)

	for _, doc := range VectorIndex {
		score := CosineSimilarity(queryVec, doc.Data)
		if score > threshold {
			if currentBest, exists := fileScores[doc.FileID]; !exists || score > currentBest {
				fileScores[doc.FileID] = score
			}
		}
	}

	var matches []Match
	for id, score := range fileScores {
		matches = append(matches, Match{FileID: id, Score: score})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > 10 {
		matches = matches[:10]
	}

	var results []SearchResult
	for _, m := range matches {
		var path, summary string
		var modTime int64

		// FETCH MODIFIED TIME to check filter
		err := DB.QueryRow("SELECT path, summary, modified_time FROM files WHERE id = ?", m.FileID).Scan(&path, &summary, &modTime)
		if err != nil {
			continue
		}

		// --- DATE FILTER CHECK ---
		if minTime > 0 && modTime < minTime {
			continue
		}
		if maxTime > 0 && modTime > maxTime {
			continue
		}
		// -------------------------

		displaySnippet := summary
		if len(displaySnippet) > 200 {
			displaySnippet = displaySnippet[:200] + "..."
		}
		results = append(results, SearchResult{
			Path:    path,
			Snippet: displaySnippet,
			Score:   m.Score,
		})
	}

	return results, nil
}
