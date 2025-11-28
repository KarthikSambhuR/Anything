package core

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

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
	file, err := os.Open(GetDataPath("vocab.txt"))
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

// Tokenize implements a simple WordPiece-style tokenizer
func Tokenize(text string) []int64 {
	text = strings.ToLower(text)

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
		}
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	var ids []int64
	ids = append(ids, TokenCLS)

	for _, word := range tokens {
		if id, ok := Vocab[word]; ok {
			ids = append(ids, id)
			continue
		}
		// Fallback to UNK (Simplified vs full WordPiece splitting)
		ids = append(ids, TokenUNK)
	}

	ids = append(ids, TokenSEP)

	// Truncate to 512 (Model Limit)
	if len(ids) > 512 {
		ids = ids[:511]
		ids = append(ids, TokenSEP)
	}

	return ids
}
