package core

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

var (
	ORT_Session            *onnxruntime_go.DynamicSession[int64, float32]
	ORT_GemmaTextSession   *onnxruntime_go.DynamicAdvancedSession
	ORT_GemmaVisionSession *onnxruntime_go.DynamicAdvancedSession
	IsAIReady              bool = false
)

const (
	ModelUrl     = "https://huggingface.co/optimum/all-MiniLM-L6-v2/resolve/main/model.onnx"
	VocabUrl     = "https://huggingface.co/optimum/all-MiniLM-L6-v2/raw/main/vocab.txt"
	OnnxWinUrl   = "https://github.com/microsoft/onnxruntime/releases/download/v1.31.0/onnxruntime-win-x64-1.31.0.zip"
	OnnxLinuxUrl = "https://github.com/microsoft/onnxruntime/releases/download/v1.31.0/onnxruntime-linux-x64-1.31.0.tgz"
	OnnxMacUrl   = "https://github.com/microsoft/onnxruntime/releases/download/v1.31.0/onnxruntime-osx-universal2-1.31.0.tgz"

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
		{"onnx/model_quantized.onnx", "model_quantized.onnx", "Gemma 2 Text Model Graph"},
		{"onnx/model_quantized.onnx_data", "model_quantized.onnx_data", "Gemma 2 Text Model Weights"},
		{"onnx/vision_encoder_quantized.onnx", "vision_encoder_quantized.onnx", "Gemma 2 Vision Encoder Graph"},
		{"onnx/vision_encoder_quantized.onnx_data", "vision_encoder_quantized.onnx_data", "Gemma 2 Vision Weights"},
		{"onnx/audio_encoder_quantized.onnx", "audio_encoder_quantized.onnx", "Gemma 2 Audio Encoder Graph"},
		{"onnx/audio_encoder_quantized.onnx_data", "audio_encoder_quantized.onnx_data", "Gemma 2 Audio Weights"},
		{"tokenizer.json", "gemma_tokenizer.json", "Gemma 2 Tokenizer"},
		{"processor_config.json", "processor_config.json", "Gemma 2 Processor Config"},
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
			ID:          "embeddinggemma-2-q4",
			Name:        "EmbeddingGemma 2 (Q4 Multimodal)",
			Description: "Google's unified multimodal model (768-d). Natively indexes text, images, video, and audio in one vector space.",
			Downloaded:  IsModelDownloaded("embeddinggemma-2-q4"),
			SizeBytes:   480 * 1024 * 1024,
		},
		{
			ID:          "all-minilm-l6-v2",
			Name:        "all-MiniLM-L6-v2",
			Description: "Ultra-fast CPU text embeddings (384-d, ~90 MB). Ideal for quick document search.",
			Downloaded:  IsModelDownloaded("all-minilm-l6-v2"),
			SizeBytes:   90 * 1024 * 1024,
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

	libName := getLibName()
	if _, err := os.Stat(libName); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] %s not found. Auto-downloading...\n", libName)
		if err := downloadOnnxRuntime(libName); err != nil {
			fmt.Printf("❌ [AI Error] Download failed: %v\n", err)
			return
		}
		fmt.Println("✅ [AI] Runtime library downloaded.")
	}

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

	currentModel := CurrentSettings.EmbeddingModel
	if currentModel == "" {
		currentModel = "embeddinggemma-2-q4"
		CurrentSettings.EmbeddingModel = currentModel
	}

	if currentModel == "embeddinggemma-2-q4" {
		if !IsModelDownloaded("embeddinggemma-2-q4") {
			fmt.Println("⚠️  [AI] EmbeddingGemma 2 files missing. Auto-downloading now...")
			if err := DownloadModelByID("embeddinggemma-2-q4"); err != nil {
				fmt.Printf("❌ [AI Error] Gemma download failed: %v\n", err)
				return
			}
		}

		fmt.Println("✅ [AI] EmbeddingGemma 2 detected. Loading Text and Vision graphs...")

		origDir, _ := os.Getwd()
		appDir := GetDataDir()
		_ = os.Chdir(appDir)
		defer os.Chdir(origDir)

		visionModelPath := GetDataPath("vision_encoder_quantized.onnx")
		textModelPath := GetDataPath("model_quantized.onnx")

		visionSession, errVision := onnxruntime_go.NewDynamicAdvancedSession(
			visionModelPath,
			[]string{"pixel_values", "pixel_position_ids"},
			[]string{"image_features"},
			nil,
		)
		if errVision != nil {
			fmt.Printf("❌ [AI Error] Failed to load Gemma Vision model: %v\n", errVision)
			return
		}

		var textSession *onnxruntime_go.DynamicAdvancedSession
		var errText error

		// Determine target GPU adapter (discrete NVIDIA/AMD GPU preferred over integrated Intel GPU)
		targetDeviceID := CurrentSettings.GpuDeviceID
		gpuName := "Auto"
		if targetDeviceID < 0 {
			targetDeviceID, gpuName = detectDedicatedGpuDeviceID()
		} else {
			gpuName = fmt.Sprintf("Adapter %d", targetDeviceID)
		}

		fmt.Printf("[AI] Targeting GPU Adapter %d: %s\n", targetDeviceID, gpuName)

		// Attempt GPU acceleration via DirectML on selected adapter
		gpuOpts, errGpu := onnxruntime_go.NewSessionOptions()
		if errGpu == nil {
			_ = gpuOpts.SetGraphOptimizationLevel(onnxruntime_go.GraphOptimizationLevelEnableAll)
			_ = gpuOpts.SetIntraOpNumThreads(4)
			if errDml := gpuOpts.AppendExecutionProviderDirectML(targetDeviceID); errDml == nil {
				textSession, errText = onnxruntime_go.NewDynamicAdvancedSession(
					textModelPath,
					[]string{"input_ids", "attention_mask", "image_features", "video_features", "audio_features"},
					[]string{"sentence_embedding"},
					gpuOpts,
				)
				if errText == nil {
					fmt.Printf("🚀 [AI] Gemma 2 Text Model running on GPU (DirectML - %s [Device %d])!\n", gpuName, targetDeviceID)
				} else {
					fmt.Printf("⚠️  [AI] DirectML GPU initialization failed on device %d: %v. Using CPU.\n", targetDeviceID, errText)
				}
			} else {
				fmt.Printf("⚠️  [AI] DirectML EP creation failed on device %d: %v. Using CPU.\n", targetDeviceID, errDml)
			}
			gpuOpts.Destroy()
		}

		if textSession == nil {
			textSession, errText = onnxruntime_go.NewDynamicAdvancedSession(
				textModelPath,
				[]string{"input_ids", "attention_mask", "image_features", "video_features", "audio_features"},
				[]string{"sentence_embedding"},
				nil,
			)
			if errText != nil {
				fmt.Printf("❌ [AI Error] Failed to load Gemma Text model: %v\n", errText)
				visionSession.Destroy()
				return
			}
			fmt.Println("✅ [AI] Gemma 2 Text Model running on CPU.")
		}

		ORT_GemmaVisionSession = visionSession
		ORT_GemmaTextSession = textSession
		IsAIReady = true
		fmt.Println("✅ [AI] Gemma 2 Multimodal Engine Ready (Text + Vision).")
		return
	}

	// MiniLM path
	modelPath := getModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] Downloading model to %s...\n", modelPath)
		if err := DownloadFileWithLabel(ModelUrl, modelPath, "MiniLM Model"); err != nil {
			fmt.Printf("❌ [AI Error] Model download failed: %v\n", err)
			return
		}
	}

	vocabPath := getVocabPath()
	if _, err := os.Stat(vocabPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  [AI] Downloading vocab to %s...\n", vocabPath)
		if err := DownloadFileWithLabel(VocabUrl, vocabPath, "MiniLM Vocab"); err != nil {
			fmt.Printf("❌ [AI Error] Vocab download failed: %v\n", err)
			return
		}
	}

	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	session, err := onnxruntime_go.NewDynamicSession[int64, float32](
		modelPath,
		inputNames,
		outputNames,
	)
	if err != nil {
		fmt.Printf("❌ [AI Error] Failed to load MiniLM model: %v\n", err)
		return
	}

	ORT_Session = session
	IsAIReady = true
	fmt.Println("✅ [AI] MiniLM Engine Ready.")
}

func CloseAI() {
	if ORT_Session != nil {
		ORT_Session.Destroy()
	}
	if ORT_GemmaTextSession != nil {
		ORT_GemmaTextSession.Destroy()
	}
	if ORT_GemmaVisionSession != nil {
		ORT_GemmaVisionSession.Destroy()
	}
	onnxruntime_go.DestroyEnvironment()
}

func detectDedicatedGpuDeviceID() (int, string) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	out, err := cmd.Output()
	if err != nil {
		return 0, "Default GPU"
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var gpus []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			gpus = append(gpus, trimmed)
		}
	}

	// Look for discrete GPU (NVIDIA / AMD Radeon / Intel Arc)
	for idx, name := range gpus {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "nvidia") || strings.Contains(lower, "geforce") ||
			strings.Contains(lower, "rtx") || strings.Contains(lower, "gtx") ||
			strings.Contains(lower, "radeon") || strings.Contains(lower, "arc") {
			return idx, name
		}
	}

	if len(gpus) > 0 {
		return 0, gpus[0]
	}
	return 0, "Default GPU"
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
	batch, err := GetBatchEmbeddings([]string{text})
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return batch[0], nil
}

func GetBatchEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if !IsAIReady {
		return nil, fmt.Errorf("AI engine not ready")
	}

	isGemma := CurrentSettings.EmbeddingModel == "embeddinggemma-2-q4"
	if !isGemma {
		if ORT_Session == nil {
			return nil, fmt.Errorf("MiniLM session not loaded")
		}
		var results [][]float32
		for _, text := range texts {
			vec, err := getSingleMiniLMEmbedding(text)
			if err != nil {
				return nil, err
			}
			results = append(results, vec)
		}
		return results, nil
	}

	if ORT_GemmaTextSession == nil {
		return nil, fmt.Errorf("Gemma text session not loaded")
	}

	batchSize := len(texts)
	tokenized := make([][]int64, batchSize)
	maxLen := int64(8)

	for i, t := range texts {
		tokens := Tokenize(t)
		if int64(len(tokens)) > maxLen {
			maxLen = int64(len(tokens))
		}
		tokenized[i] = tokens
	}

	if maxLen > 512 {
		maxLen = 512
	}

	totalTokens := int64(batchSize) * maxLen
	inputIDs := make([]int64, totalTokens)
	attentionMask := make([]int64, totalTokens)

	for b := 0; b < batchSize; b++ {
		tokens := tokenized[b]
		rowLen := int64(len(tokens))
		if rowLen > maxLen {
			rowLen = maxLen
		}
		rowOffset := int64(b) * maxLen
		for s := int64(0); s < rowLen; s++ {
			inputIDs[rowOffset+s] = tokens[s]
			attentionMask[rowOffset+s] = 1
		}
	}

	tInput, err := onnxruntime_go.NewTensor([]int64{int64(batchSize), maxLen}, inputIDs)
	if err != nil {
		return nil, err
	}
	defer tInput.Destroy()

	tMask, err := onnxruntime_go.NewTensor([]int64{int64(batchSize), maxLen}, attentionMask)
	if err != nil {
		return nil, err
	}
	defer tMask.Destroy()

	tEmptyImg, _ := onnxruntime_go.NewTensor([]int64{0, 512}, []float32{})
	defer tEmptyImg.Destroy()
	tEmptyVid, _ := onnxruntime_go.NewTensor([]int64{0, 512}, []float32{})
	defer tEmptyVid.Destroy()
	tEmptyAud, _ := onnxruntime_go.NewTensor([]int64{0, 512}, []float32{})
	defer tEmptyAud.Destroy()

	tOutput, err := onnxruntime_go.NewEmptyTensor[float32]([]int64{int64(batchSize), 768})
	if err != nil {
		return nil, err
	}
	defer tOutput.Destroy()

	err = ORT_GemmaTextSession.Run(
		[]onnxruntime_go.Value{tInput, tMask, tEmptyImg, tEmptyVid, tEmptyAud},
		[]onnxruntime_go.Value{tOutput},
	)
	if err != nil {
		return nil, fmt.Errorf("Gemma batched text run error: %w", err)
	}

	outputData := tOutput.GetData()
	results := make([][]float32, batchSize)

	for b := 0; b < batchSize; b++ {
		row := make([]float32, 768)
		copy(row, outputData[b*768:(b+1)*768])

		var sumSquares float32
		for _, val := range row {
			sumSquares += val * val
		}
		norm := float32(math.Sqrt(float64(sumSquares)))
		if norm == 0 {
			norm = 1e-9
		}
		for j := 0; j < 768; j++ {
			row[j] /= norm
		}
		results[b] = row
	}

	return results, nil
}

func getSingleMiniLMEmbedding(text string) ([]float32, error) {

	// MiniLM
	if ORT_Session == nil {
		return nil, fmt.Errorf("MiniLM session not loaded")
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

	shape := []int64{1, seqLength}
	tInput, _ := onnxruntime_go.NewTensor(shape, inputIDs)
	defer tInput.Destroy()
	tMask, _ := onnxruntime_go.NewTensor(shape, attentionMask)
	defer tMask.Destroy()
	tType, _ := onnxruntime_go.NewTensor(shape, tokenTypeIDs)
	defer tType.Destroy()

	hiddenSize := 384
	outputShape := []int64{1, seqLength, int64(hiddenSize)}
	outputData := make([]float32, 1*seqLength*int64(hiddenSize))
	tOutput, _ := onnxruntime_go.NewTensor(outputShape, outputData)
	defer tOutput.Destroy()

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

	// L2 Normalize
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

// preprocessGemmaImage converts an arbitrary image into Gemma 4 vision patches and position IDs
func preprocessGemmaImage(img image.Image) ([]float32, []int64) {
	const maxPatches = 280
	const patchSize = 16
	const poolingKernel = 3
	const sideMult = poolingKernel * patchSize // 48

	origB := img.Bounds()
	origW := origB.Dx()
	origH := origB.Dy()

	targetPx := float64(maxPatches * patchSize * patchSize)
	factor := math.Sqrt(targetPx / float64(origW*origH))
	idealH := factor * float64(origH)
	idealW := factor * float64(origW)

	targetH := int(math.Floor(idealH/float64(sideMult))) * sideMult
	targetW := int(math.Floor(idealW/float64(sideMult))) * sideMult

	if targetH == 0 {
		targetH = sideMult
	}
	if targetW == 0 {
		targetW = sideMult
	}

	// Resize image
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.BiLinear.Scale(dst, dst.Rect, img, origB, draw.Over, nil)

	patchesY := targetH / patchSize
	patchesX := targetW / patchSize

	pixelValues := make([]float32, maxPatches*768)
	posIDs := make([]int64, maxPatches*2)

	// Set pad positions to -1
	for i := 0; i < maxPatches*2; i++ {
		posIDs[i] = -1
	}

	patchIdx := 0
	for py := 0; py < patchesY; py++ {
		for px := 0; px < patchesX; px++ {
			if patchIdx >= maxPatches {
				break
			}
			posIDs[patchIdx*2] = int64(py)
			posIDs[patchIdx*2+1] = int64(px)

			patchOffset := patchIdx * 768
			pixelInPatch := 0
			for y := 0; y < patchSize; y++ {
				for x := 0; x < patchSize; x++ {
					imgX := px*patchSize + x
					imgY := py*patchSize + y
					r, g, b, _ := dst.At(imgX, imgY).RGBA()

					// Rescale to [0, 1]
					pixelValues[patchOffset+pixelInPatch*3+0] = float32(r>>8) / 255.0
					pixelValues[patchOffset+pixelInPatch*3+1] = float32(g>>8) / 255.0
					pixelValues[patchOffset+pixelInPatch*3+2] = float32(b>>8) / 255.0
					pixelInPatch++
				}
			}
			patchIdx++
		}
	}

	return pixelValues, posIDs
}

func GetImageEmbedding(path string) ([]float32, error) {
	if !IsAIReady || ORT_GemmaVisionSession == nil || ORT_GemmaTextSession == nil {
		return nil, fmt.Errorf("Gemma Multimodal engine not ready")
	}

	img, err := loadImage(path)
	if err != nil {
		return nil, fmt.Errorf("failed loading image: %w", err)
	}

	pixVals, posIDs := preprocessGemmaImage(img)

	tPixVals, err := onnxruntime_go.NewTensor([]int64{1, 280, 768}, pixVals)
	if err != nil {
		return nil, err
	}
	defer tPixVals.Destroy()

	tPosIDs, err := onnxruntime_go.NewTensor([]int64{1, 280, 2}, posIDs)
	if err != nil {
		return nil, err
	}
	defer tPosIDs.Destroy()

	visionOutputs := []onnxruntime_go.Value{nil}
	err = ORT_GemmaVisionSession.Run(
		[]onnxruntime_go.Value{tPixVals, tPosIDs},
		visionOutputs,
	)
	if err != nil {
		return nil, fmt.Errorf("Gemma Vision run error: %w", err)
	}
	defer visionOutputs[0].Destroy()

	tVisionOut := visionOutputs[0].(*onnxruntime_go.Tensor[float32])
	visFeats := tVisionOut.GetData()
	visShape := tVisionOut.GetShape()
	numImgTokens := visShape[0]

	// Multimodal merge through Gemma text model: <boi> + numImgTokens * <image> + <eoi>
	var imgTokens []int64
	imgTokens = append(imgTokens, GemmaTokenBOI)
	for j := int64(0); j < numImgTokens; j++ {
		imgTokens = append(imgTokens, GemmaTokenImage)
	}
	imgTokens = append(imgTokens, GemmaTokenEOI)

	seqLen := int64(len(imgTokens))
	mask := make([]int64, seqLen)
	for j := range mask {
		mask[j] = 1
	}

	tTokens, err := onnxruntime_go.NewTensor([]int64{1, seqLen}, imgTokens)
	if err != nil {
		return nil, err
	}
	defer tTokens.Destroy()

	tMask, err := onnxruntime_go.NewTensor([]int64{1, seqLen}, mask)
	if err != nil {
		return nil, err
	}
	defer tMask.Destroy()

	tImgFeats, err := onnxruntime_go.NewTensor([]int64{numImgTokens, 512}, visFeats)
	if err != nil {
		return nil, err
	}
	defer tImgFeats.Destroy()

	tEmptyVid, _ := onnxruntime_go.NewTensor([]int64{0, 512}, []float32{})
	defer tEmptyVid.Destroy()
	tEmptyAud, _ := onnxruntime_go.NewTensor([]int64{0, 512}, []float32{})
	defer tEmptyAud.Destroy()

	tOut, err := onnxruntime_go.NewEmptyTensor[float32]([]int64{1, 768})
	if err != nil {
		return nil, err
	}
	defer tOut.Destroy()

	err = ORT_GemmaTextSession.Run(
		[]onnxruntime_go.Value{tTokens, tMask, tImgFeats, tEmptyVid, tEmptyAud},
		[]onnxruntime_go.Value{tOut},
	)
	if err != nil {
		return nil, fmt.Errorf("Gemma Multimodal merge error: %w", err)
	}

	rawVec := tOut.GetData()
	embedding := make([]float32, 768)
	copy(embedding, rawVec[:768])

	// L2 Normalize
	var sumSquares float32
	for _, val := range embedding {
		sumSquares += val * val
	}
	norm := float32(math.Sqrt(float64(sumSquares)))
	if norm == 0 {
		norm = 1e-9
	}
	for j := 0; j < 768; j++ {
		embedding[j] /= norm
	}

	return embedding, nil
}
