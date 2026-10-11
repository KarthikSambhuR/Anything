package core

import (
	"encoding/binary"
	"fmt"
	"math"
)



func LoadVectorIndex() {
	fmt.Print("Vector Index is now managed natively by sqlite-vec in DB.\n")
}

func SemanticSearch(query string, minTime int64, maxTime int64) ([]SearchResult, error) {
	if !IsAIReady {
		return nil, fmt.Errorf("AI not ready")
	}

	queryVec, err := GetEmbedding(query)
	if err != nil {
		return nil, err
	}

	byteBuf := make([]byte, len(queryVec)*4)
	for i, v := range queryVec {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(byteBuf[i*4:], bits)
	}

	var tableName string
	if len(queryVec) == 384 {
		tableName = "vec_minilm"
	} else if len(queryVec) == 768 {
		tableName = "vec_gemma"
	} else {
		return nil, fmt.Errorf("unsupported embedding size")
	}

	// Fetch up to 20 to allow time filtering without dropping below 10 if some are filtered
	queryStr := fmt.Sprintf(`
		SELECT f.path, f.summary, f.modified_time, COALESCE(f.icon_data, ''), f.extension, v.distance
		FROM %s v
		JOIN file_vectors fv ON fv.id = v.rowid
		JOIN files f ON f.id = fv.file_id
		WHERE v.embedding MATCH ? AND k = 20
	`, tableName)

	rows, err := DB.Query(queryStr, byteBuf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var path, summary, iconData, extension string
		var modTime int64
		var distance float32

		if err := rows.Scan(&path, &summary, &modTime, &iconData, &extension, &distance); err != nil {
			continue
		}

		if minTime > 0 && modTime < minTime {
			continue
		}
		if maxTime > 0 && modTime > maxTime {
			continue
		}

		displaySnippet := summary
		if displaySnippet == "[IMAGE_MULTIMODAL]" {
			displaySnippet = "Visual Content (Gemma Multimodal)"
		} else if len(displaySnippet) > 200 {
			displaySnippet = displaySnippet[:200] + "..."
		}

		results = append(results, SearchResult{
			Path:      path,
			Snippet:   displaySnippet,
			Score:     1.0 - distance, // Approximation for score
			IconData:  iconData,
			Extension: extension,
		})

		if len(results) >= 10 {
			break
		}
	}
	return results, nil
}
