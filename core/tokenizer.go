package core

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Global Vocab Map
var Vocab map[string]int64

// Special Tokens for BERT
const (
	TokenCLS = 101 // [CLS] Start of sequence
	TokenSEP = 102 // [SEP] Separator
	TokenUNK = 100 // [UNK] Unknown word
	TokenPAD = 0   // [PAD] Padding
)

func InitTokenizer() {
	fmt.Println("[AI] Loading Tokenizer Vocab...")
	file, err := os.Open("vocab.txt")
	if err != nil {
		fmt.Printf("❌ [AI Error] Could not load vocab.txt: %v\n", err)
		return
	}
	defer file.Close()

	Vocab = make(map[string]int64)
	scanner := bufio.NewScanner(file)
	var id int64 = 0
	for scanner.Scan() {
		line := scanner.Text()
		Vocab[line] = id
		id++
	}
	fmt.Printf("✅ [AI] Tokenizer Ready (%d words).\n", len(Vocab))
}

// Simple WordPiece Tokenizer
func Tokenize(text string) []int64 {
	// 1. Clean and Lowercase
	text = strings.ToLower(text)

	// 2. Simple split by whitespace and punctuation
	// This is a simplified version of BERT BasicTokenizer
	var tokens []string
	var currentToken strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			currentToken.WriteRune(r)
		} else {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			// Treat punctuation as separate token?
			// For simplicity in v0.3, we ignore punctuation to match "Simple" mode
		}
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	// 3. Convert to IDs (WordPiece greedy matching)
	var ids []int64
	ids = append(ids, TokenCLS) // Start with [CLS]

	for _, word := range tokens {
		// Try to find the word in vocab
		if id, ok := Vocab[word]; ok {
			ids = append(ids, id)
			continue
		}

		// If not found, try to break it down?
		// For "Lightweight" v0.3, if it's not in vocab, we map to [UNK]
		// Implementing full WordPiece sub-word splitting (play -> ##ing) is complex logic
		// For now, we accept whole words or UNK.
		ids = append(ids, TokenUNK)
	}

	ids = append(ids, TokenSEP) // End with [SEP]

	// Truncate to 512 (Model Limit)
	if len(ids) > 512 {
		ids = ids[:511]
		ids = append(ids, TokenSEP)
	}

	return ids
}
