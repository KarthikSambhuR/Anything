package core

import (
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/yalue/onnxruntime_go"
)

// AI Globals
var (
	ORT_Session *onnxruntime_go.DynamicSession[int64, float32]
	IsAIReady   bool = false
)

// Constants
const (
	ModelPath = "model.onnx"
	VocabPath = "vocab.txt"

	ModelUrl = "https://huggingface.co/optimum/all-MiniLM-L6-v2/resolve/main/model.onnx"
	VocabUrl = "https://huggingface.co/optimum/all-MiniLM-L6-v2/blob/main/vocab.txt"

	// UPDATED: Bumped to v1.20.1 to match your Go library version
	OnnxWinUrl   = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-win-x64-1.22.0.zip"
	OnnxLinuxUrl = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-linux-x64-1.22.0.tgz"
	OnnxMacUrl   = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-osx-universal2-1.22.0.tgz"
)

func getLibPath() string {
	if runtime.GOOS == "windows" {
		return "onnxruntime.dll"
	}
	if runtime.GOOS == "darwin" {
		return "libonnxruntime.dylib"
	}
	return "libonnxruntime.so"
}

func InitAI() {
	fmt.Println("[AI] Initializing ONNX Runtime...")
	libPath := getLibPath()

	// 1. Check & Download ONNX Runtime Library
	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] %s not found. Auto-downloading...\n", libPath)
		if err := downloadOnnxRuntime(libPath); err != nil {
			fmt.Printf("❌ [AI Error] Download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Runtime library downloaded.")
	}

	// 2. Initialize Environment
	onnxruntime_go.SetSharedLibraryPath(libPath)
	err := onnxruntime_go.InitializeEnvironment()
	if err != nil {
		fmt.Printf("❌ [AI Error] Failed to init ONNX Environment: %v\n", err)
		return
	}

	// 3. Check & Download Model
	if _, err := os.Stat(ModelPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] %s not found. Auto-downloading...\n", ModelPath)
		if err := DownloadFile(ModelUrl, ModelPath); err != nil {
			fmt.Printf("❌ [AI Error] Model download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Model downloaded.")
	}

	// 4. Check & Download Vocab
	if _, err := os.Stat(VocabPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] %s not found. Auto-downloading...\n", VocabPath)
		if err := DownloadFile(VocabUrl, VocabPath); err != nil {
			fmt.Printf("❌ [AI Error] Vocab download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Vocab downloaded.")
	}

	// 5. Create Session
	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	session, err := onnxruntime_go.NewDynamicSession[int64, float32](
		ModelPath,
		inputNames,
		outputNames,
	)
	if err != nil {
		fmt.Printf("❌ [AI Error] Failed to load model: %v\n", err)
		return
	}

	ORT_Session = session
	IsAIReady = true
	fmt.Println("✅ [AI] Semantic Engine Ready.")
}

func CloseAI() {
	if ORT_Session != nil {
		ORT_Session.Destroy()
	}
	onnxruntime_go.DestroyEnvironment()
}

// Logic to download and extract the correct DLL/SO based on OS
func downloadOnnxRuntime(targetLibName string) error {
	var downloadUrl string
	var archiveName string

	switch runtime.GOOS {
	case "windows":
		downloadUrl = OnnxWinUrl
		archiveName = "onnx_temp.zip"
	case "linux":
		return fmt.Errorf("manual download required for Linux in v0.3")
	default:
		return fmt.Errorf("unsupported OS for auto-download")
	}

	fmt.Printf("Downloading %s from %s...\n", archiveName, downloadUrl)
	if err := DownloadFile(downloadUrl, archiveName); err != nil {
		return err
	}

	fmt.Println("Extracting library...")
	if err := ExtractFileFromZip(archiveName, targetLibName, targetLibName); err != nil {
		return err
	}

	os.Remove(archiveName)
	return nil
}

// --- EMBEDDING ENGINE ---

// --- EMBEDDING ENGINE ---

func GetEmbedding(text string) ([]float32, error) {
	if !IsAIReady || ORT_Session == nil {
		return nil, fmt.Errorf("AI engine not ready")
	}

	// 1. Tokenize
	inputIDSlice := Tokenize(text)
	seqLength := int64(len(inputIDSlice))

	// 2. Prepare Inputs
	inputIDs := make([]int64, seqLength)
	attentionMask := make([]int64, seqLength)
	tokenTypeIDs := make([]int64, seqLength)

	for i, id := range inputIDSlice {
		inputIDs[i] = id
		attentionMask[i] = 1
		tokenTypeIDs[i] = 0
	}

	shape := []int64{1, seqLength}
	tInput, _ := onnxruntime_go.NewTensor(shape, inputIDs)
	tMask, _ := onnxruntime_go.NewTensor(shape, attentionMask)
	tType, _ := onnxruntime_go.NewTensor(shape, tokenTypeIDs)

	// 3. Prepare Output (The FIX)
	// MiniLM output is [1, seqLength, 384]
	hiddenSize := 384
	outputShape := []int64{1, seqLength, int64(hiddenSize)}
	// Allocate the flat array to hold the result
	outputData := make([]float32, 1*seqLength*int64(hiddenSize))

	tOutput, _ := onnxruntime_go.NewTensor(outputShape, outputData)

	// 4. Run Inference
	// Pass inputs AND outputs. Returns only error.
	err := ORT_Session.Run(
		[]*onnxruntime_go.Tensor[int64]{tInput, tMask, tType},
		[]*onnxruntime_go.Tensor[float32]{tOutput},
	)
	if err != nil {
		return nil, err
	}

	// 5. Mean Pooling
	// The 'outputData' slice is now filled with numbers by ONNX.
	embedding := make([]float32, hiddenSize)

	for i := 0; i < int(seqLength); i++ {
		start := i * hiddenSize
		for j := 0; j < hiddenSize; j++ {
			embedding[j] += outputData[start+j]
		}
	}

	// Average
	for j := 0; j < hiddenSize; j++ {
		embedding[j] /= float32(seqLength)
	}

	// 6. Normalize
	var sumSquares float32
	for _, val := range embedding {
		sumSquares += val * val
	}
	norm := float32(math.Sqrt(float64(sumSquares)))
	if norm == 0 {
		norm = 1e-9
	}

	for j := 0; j < hiddenSize; j++ {
		embedding[j] /= norm
	}

	return embedding, nil
}
