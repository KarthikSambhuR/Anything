package core

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"
)

type CachedVector struct {
	FileID     int
	ChunkIndex int
	Data       []float32
}

var VectorIndex []CachedVector

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

func LoadVectorIndex() {
	fmt.Print("Loading Vector Index into RAM... ")
	startTime := time.Now()
	rows, err := DB.Query("SELECT file_id, chunk_index, vector_blob FROM file_vectors")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer rows.Close()

	VectorIndex = []CachedVector{}
	for rows.Next() {
		var fileID, chunkIdx int
		var blob []byte
		if err := rows.Scan(&fileID, &chunkIdx, &blob); err != nil {
			continue
		}
		vecLen := len(blob) / 4
		vec := make([]float32, vecLen)
		for i := 0; i < vecLen; i++ {
			bits := binary.LittleEndian.Uint32(blob[i*4 : (i+1)*4])
			vec[i] = math.Float32frombits(bits)
		}
		VectorIndex = append(VectorIndex, CachedVector{FileID: fileID, ChunkIndex: chunkIdx, Data: vec})
	}
	fmt.Printf("Done! Loaded %d vectors in %v\n", len(VectorIndex), time.Since(startTime))
}

func SemanticSearch(query string, minTime int64, maxTime int64) ([]SearchResult, error) {
	if !IsAIReady || len(VectorIndex) == 0 {
		return nil, fmt.Errorf("AI not ready")
	}

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

	// Brute-force Cosine Similarity against RAM index
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
		var path, summary, iconData, extension string
		var modTime int64

		err := DB.QueryRow("SELECT path, summary, modified_time, COALESCE(icon_data, ''), extension FROM files WHERE id = ?", m.FileID).Scan(&path, &summary, &modTime, &iconData, &extension)
		if err != nil {
			continue
		}

		if minTime > 0 && modTime < minTime {
			continue
		}
		if maxTime > 0 && modTime > maxTime {
			continue
		}

		displaySnippet := summary
		if len(displaySnippet) > 200 {
			displaySnippet = displaySnippet[:200] + "..."
		}

		results = append(results, SearchResult{
			Path:      path,
			Snippet:   displaySnippet,
			Score:     m.Score,
			IconData:  iconData,
			Extension: extension,
		})
	}
	return results, nil
}
