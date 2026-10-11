package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

var (
	Vocab      map[string]int64
	GemmaVocab map[string]int64
)

// Special Tokens for BERT
const (
	TokenCLS = 101 // [CLS] Start of sequence
	TokenSEP = 102 // [SEP] Separator
	TokenUNK = 100 // [UNK] Unknown word
	TokenPAD = 0   // [PAD] Padding
)

// Special Tokens for Gemma 2
const (
	GemmaTokenPAD   = 0
	GemmaTokenEOS   = 1
	GemmaTokenBOS   = 2
	GemmaTokenUNK   = 3
	GemmaTokenBOI   = 255999
	GemmaTokenEOI   = 258882
	GemmaTokenImage = 258880
)

type gemmaTokenizerFile struct {
	Model struct {
		Vocab map[string]int64 `json:"vocab"`
	} `json:"model"`
}

func InitTokenizer() {
	if CurrentSettings.EmbeddingModel == "embeddinggemma-2-q4" {
		tokPath := GetDataPath("gemma_tokenizer.json")
		if info, err := os.Stat(tokPath); err == nil && info.Size() > 0 {
			fmt.Println("[AI] Loading Gemma Tokenizer Vocab...")
			file, err := os.Open(tokPath)
			if err == nil {
				defer file.Close()
				var tf gemmaTokenizerFile
				if err := json.NewDecoder(file).Decode(&tf); err == nil && len(tf.Model.Vocab) > 0 {
					GemmaVocab = tf.Model.Vocab
					fmt.Printf("✅ [AI] Gemma Tokenizer Ready (%d tokens).\n", len(GemmaVocab))
					return
				}
			}
		}
	}

	fmt.Println("[AI] Loading MiniLM Tokenizer Vocab...")
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

func tokenizeWordGemma(word string) []int64 {
	if id, ok := GemmaVocab[word]; ok {
		return []int64{id}
	}
	var res []int64
	runes := []rune(word)
	for i := 0; i < len(runes); {
		matched := false
		for j := len(runes); j > i; j-- {
			sub := string(runes[i:j])
			if id, ok := GemmaVocab[sub]; ok {
				res = append(res, id)
				i = j
				matched = true
				break
			}
		}
		if !matched {
			res = append(res, GemmaTokenUNK)
			i++
		}
	}
	return res
}

// Tokenize implements tokenization for both Gemma 2 and BERT/MiniLM
func Tokenize(text string) []int64 {
	text = strings.ToLower(text)

	var words []string
	var currentToken strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			currentToken.WriteRune(r)
		} else {
			if currentToken.Len() > 0 {
				words = append(words, currentToken.String())
				currentToken.Reset()
			}
		}
	}
	if currentToken.Len() > 0 {
		words = append(words, currentToken.String())
	}

	if CurrentSettings.EmbeddingModel == "embeddinggemma-2-q4" && len(GemmaVocab) > 0 {
		var ids []int64
		ids = append(ids, GemmaTokenBOS)

		for _, word := range words {
			ids = append(ids, tokenizeWordGemma(word)...)
		}

		ids = append(ids, GemmaTokenEOS)

		// Gemma 2 supports 4096+ tokens, truncate to 2048 for search
		if len(ids) > 2048 {
			ids = ids[:2047]
			ids = append(ids, GemmaTokenEOS)
		}
		return ids
	}

	// Default: MiniLM WordPiece
	var ids []int64
	ids = append(ids, TokenCLS)

	for _, word := range words {
		if id, ok := Vocab[word]; ok {
			ids = append(ids, id)
			continue
		}
		ids = append(ids, TokenUNK)
	}

	ids = append(ids, TokenSEP)

	if len(ids) > 512 {
		ids = ids[:511]
		ids = append(ids, TokenSEP)
	}

	return ids
}
