package core

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/yalue/onnxruntime_go"
)

var (
	ORT_Session *onnxruntime_go.DynamicSession[int64, float32]
	IsAIReady   bool = false
)

const (
	ModelUrl     = "https://huggingface.co/optimum/all-MiniLM-L6-v2/resolve/main/model.onnx"
	VocabUrl     = "https://huggingface.co/optimum/all-MiniLM-L6-v2/raw/main/vocab.txt"
	OnnxWinUrl   = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-win-x64-1.22.0.zip"
	OnnxLinuxUrl = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-linux-x64-1.22.0.tgz"
	OnnxMacUrl   = "https://github.com/microsoft/onnxruntime/releases/download/v1.22.0/onnxruntime-osx-universal2-1.22.0.tgz"

	GemmaBaseUrl = "https://huggingface.co/onnx-community/embeddinggemma-2-ONNX/resolve/main"
)

type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Downloaded  bool   `json:"downloaded"`
	SizeBytes   int64  `json:"size_bytes"`
}

func getLibName() string {
	if runtime.GOOS == "windows" {
		return "onnxruntime.dll"
	}
	if runtime.GOOS == "darwin" {
		return "libonnxruntime.dylib"
	}
	return "libonnxruntime.so"
}

func getModelPath() string { return GetDataPath("model.onnx") }
func getVocabPath() string { return GetDataPath("vocab.txt") }

func getGemmaFiles() []struct {
	Subpath  string
	FileName string
	Label    string
} {
	return []struct {
		Subpath  string
		FileName string
		Label    string
	}{
		{"onnx/model_q4.onnx", "gemma_text_q4.onnx", "Gemma 2 Text Model Graph"},
		{"onnx/model_q4.onnx_data", "gemma_text_q4.onnx_data", "Gemma 2 Text Model Weights"},
		{"onnx/vision_encoder_q4.onnx", "gemma_vision_q4.onnx", "Gemma 2 Vision Encoder Graph"},
		{"onnx/vision_encoder_q4.onnx_data", "gemma_vision_q4.onnx_data", "Gemma 2 Vision Weights"},
		{"onnx/audio_encoder_q4.onnx", "gemma_audio_q4.onnx", "Gemma 2 Audio Encoder Graph"},
		{"onnx/audio_encoder_q4.onnx_data", "gemma_audio_q4.onnx_data", "Gemma 2 Audio Weights"},
		{"tokenizer.json", "gemma_tokenizer.json", "Gemma 2 Tokenizer"},
		{"preprocessor_config.json", "gemma_preprocessor_config.json", "Gemma 2 Preprocessor Config"},
		{"config.json", "gemma_config.json", "Gemma 2 Architecture Config"},
	}
}

func IsModelDownloaded(modelID string) bool {
	if modelID == "embeddinggemma-2-q4" {
		for _, f := range getGemmaFiles() {
			path := GetDataPath(f.FileName)
			info, err := os.Stat(path)
			if err != nil || info.Size() == 0 {
				return false
			}
		}
		return true
	}

	// Default: all-minilm-l6-v2
	info1, err1 := os.Stat(getModelPath())
	info2, err2 := os.Stat(getVocabPath())
	return err1 == nil && info1.Size() > 0 && err2 == nil && info2.Size() > 0
}

func GetAvailableModels() []ModelInfo {
	return []ModelInfo{
		{
			ID:          "all-minilm-l6-v2",
			Name:        "all-MiniLM-L6-v2",
			Description: "Ultra-fast CPU text embeddings (384-d, ~90 MB). Ideal for quick document search.",
			Downloaded:  IsModelDownloaded("all-minilm-l6-v2"),
			SizeBytes:   90 * 1024 * 1024,
		},
		{
			ID:          "embeddinggemma-2-q4",
			Name:        "EmbeddingGemma 2 (Q4 Multimodal)",
			Description: "Google's unified multimodal model (768-d, ~480 MB). Natively indexes text, images, video, and audio in one vector space.",
			Downloaded:  IsModelDownloaded("embeddinggemma-2-q4"),
			SizeBytes:   480 * 1024 * 1024,
		},
	}
}

func DownloadModelByID(modelID string) error {
	if modelID == "embeddinggemma-2-q4" {
		files := getGemmaFiles()
		totalFiles := len(files)
		for idx, f := range files {
			destPath := GetDataPath(f.FileName)
			if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
				continue
			}

			url := fmt.Sprintf("%s/%s", GemmaBaseUrl, f.Subpath)
			label := fmt.Sprintf("[%d/%d] %s", idx+1, totalFiles, f.Label)
			EmitProgress("download", label, (idx*100)/totalFiles)
			if err := DownloadFileWithLabel(url, destPath, label); err != nil {
				return fmt.Errorf("failed downloading %s: %w", f.FileName, err)
			}
		}
		EmitProgress("download", "EmbeddingGemma 2 downloaded successfully!", 100)
		return nil
	}

	// Default MiniLM
	if _, err := os.Stat(getModelPath()); os.IsNotExist(err) {
		EmitProgress("download", "Downloading MiniLM Model...", 20)
		if err := DownloadFileWithLabel(ModelUrl, getModelPath(), "MiniLM Model"); err != nil {
			return err
		}
	}
	if _, err := os.Stat(getVocabPath()); os.IsNotExist(err) {
		EmitProgress("download", "Downloading MiniLM Vocab...", 80)
		if err := DownloadFileWithLabel(VocabUrl, getVocabPath(), "MiniLM Vocab"); err != nil {
			return err
		}
	}
	EmitProgress("download", "all-MiniLM-L6-v2 ready!", 100)
	return nil
}

func InitAI() {
	fmt.Println("[AI] Initializing ONNX Runtime...")

	// We keep the DLL in the local folder (next to .exe) to avoid Antivirus issues with AppData execution
	libName := getLibName()

	if _, err := os.Stat(libName); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] %s not found. Auto-downloading...\n", libName)
		if err := downloadOnnxRuntime(libName); err != nil {
			fmt.Printf("❌ [AI Error] Download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Runtime library downloaded.")
	}

	// Important: Use Absolute Path for DLL to prevent Windows loading a wrong version from System32
	absLibPath, err := filepath.Abs(libName)
	if err != nil {
		fmt.Printf("❌ [AI Error] Could not determine absolute path: %v\n", err)
		return
	}

	fmt.Printf("[AI] Loading DLL from: %s\n", absLibPath)
	onnxruntime_go.SetSharedLibraryPath(absLibPath)

	err = onnxruntime_go.InitializeEnvironment()
	if err != nil {
		fmt.Printf("❌ [AI Error] Failed to init ONNX Environment: %v\n", err)
		return
	}

	// Auto ensure selected model is available
	currentModel := CurrentSettings.EmbeddingModel
	if currentModel == "" {
		currentModel = "all-minilm-l6-v2"
	}

	if currentModel == "embeddinggemma-2-q4" {
		if !IsModelDownloaded("embeddinggemma-2-q4") {
			fmt.Println("⚠️  [AI] EmbeddingGemma 2 selected but not downloaded yet. Run download from Settings.")
			return
		}
		fmt.Println("✅ [AI] EmbeddingGemma 2 detected on disk.")
		IsAIReady = true
		return
	}

	modelPath := getModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] Downloading model to %s...\n", modelPath)
		if err := DownloadFileWithLabel(ModelUrl, modelPath, "MiniLM Model"); err != nil {
			fmt.Printf("❌ [AI Error] Model download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Model downloaded.")
	}

	vocabPath := getVocabPath()
	if _, err := os.Stat(vocabPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] Downloading vocab to %s...\n", vocabPath)
		if err := DownloadFileWithLabel(VocabUrl, vocabPath, "MiniLM Vocab"); err != nil {
			fmt.Printf("❌ [AI Error] Vocab download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Vocab downloaded.")
	}

	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	session, err := onnxruntime_go.NewDynamicSession[int64, float32](
		modelPath,
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

func downloadOnnxRuntime(targetLibName string) error {
	var downloadUrl string
	var archiveName string

	switch runtime.GOOS {
	case "windows":
		downloadUrl = OnnxWinUrl
		archiveName = "onnx_temp.zip"
	case "linux":
		return fmt.Errorf("manual download required for Linux in this version")
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

func GetEmbedding(text string) ([]float32, error) {
	if !IsAIReady || ORT_Session == nil {
		return nil, fmt.Errorf("AI engine not ready")
	}

	inputIDSlice := Tokenize(text)
	seqLength := int64(len(inputIDSlice))

	inputIDs := make([]int64, seqLength)
	attentionMask := make([]int64, seqLength)
	tokenTypeIDs := make([]int64, seqLength)

	for i, id := range inputIDSlice {
		inputIDs[i] = id
		attentionMask[i] = 1
		tokenTypeIDs[i] = 0
	}

	// Shape: [1, seqLength]
	shape := []int64{1, seqLength}
	tInput, _ := onnxruntime_go.NewTensor(shape, inputIDs)
	tMask, _ := onnxruntime_go.NewTensor(shape, attentionMask)
	tType, _ := onnxruntime_go.NewTensor(shape, tokenTypeIDs)

	// Prepare Output (Pre-allocated for DynamicSession)
	hiddenSize := 384
	outputShape := []int64{1, seqLength, int64(hiddenSize)}
	outputData := make([]float32, 1*seqLength*int64(hiddenSize))

	tOutput, _ := onnxruntime_go.NewTensor(outputShape, outputData)

	err := ORT_Session.Run(
		[]*onnxruntime_go.Tensor[int64]{tInput, tMask, tType},
		[]*onnxruntime_go.Tensor[float32]{tOutput},
	)
	if err != nil {
		return nil, err
	}

	// Mean Pooling
	embedding := make([]float32, hiddenSize)
	for i := 0; i < int(seqLength); i++ {
		start := i * hiddenSize
		for j := 0; j < hiddenSize; j++ {
			embedding[j] += outputData[start+j]
		}
	}

	for j := 0; j < hiddenSize; j++ {
		embedding[j] /= float32(seqLength)
	}

	// Normalize (L2)
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
