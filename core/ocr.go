package core

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"

	"context"
	"path/filepath"
	"time"

	"github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

var (
	TagSession *onnxruntime_go.DynamicSession[float32, float32]
	ImageTags  []string
	// Tiny MobileNet V2 (14 MB) - fast and standard
	UrlTag    = "https://github.com/onnx/models/raw/main/validated/vision/classification/mobilenet/model/mobilenetv2-7.onnx"
	UrlLabels = "https://raw.githubusercontent.com/onnx/models/main/validated/vision/classification/synset.txt"
)

// InitVision prepares the AI models. Call this in app.go startup.
func InitVision() {
	fmt.Println("[Vision] Initializing Vision Engine...")

	// 1. Download Model & Labels
	ensureModel("tags.onnx", UrlTag)
	ensureModel("labels.txt", UrlLabels)

	// 2. Load Labels
	loadLabels()

	// 3. Init MobileNet Session
	var err error
	// FIX: MobileNet v2 (Opset 7) uses "data" as input, not "input"
	// FIX: Output name is "mobilenetv20_output_flatten0_reshape0", not "output"
	TagSession, err = onnxruntime_go.NewDynamicSession[float32, float32](
		GetDataPath("tags.onnx"),
		[]string{"data"}, // Correct Input Name
		[]string{"mobilenetv20_output_flatten0_reshape0"}, // Correct Output Name
	)
	if err != nil {
		fmt.Printf("⚠️ [Vision] Tagging Engine failed: %v\n", err)
	} else {
		fmt.Println("✅ [Vision] Engine Ready.")
	}
}

// AnalyzeImage runs the full pipeline: MobileNet Tags + Windows OCR
func AnalyzeImage(path string) (string, error) {
	// --- FILTER STEP: Skip tiny images (Icons/UI elements) ---
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	// DecodeConfig only reads the header (fast), not the whole pixels
	config, _, err := image.DecodeConfig(f)
	f.Close() // Close immediately

	if err == nil {
		// Skip images smaller than 64x64 pixels (likely icons)
		if config.Width < 64 || config.Height < 64 {
			return "", nil
		}
	}
	// ---------------------------------------------------------

	// 1. Get Tags (Fast AI)
	tags := runTagging(path)

	// 2. Run Windows Native OCR (Powershell)
	ocrText, err := runWindowsOCR(path)
	if err != nil {
		fmt.Printf("⚠️ [Vision] OCR Engine failed: %v\n", err)
	}
	// 3. Combine for Search Index
	if len(tags) == 0 && ocrText == "" {
		return "", nil
	}

	summary := fmt.Sprintf("Tags: [%s] Content: %s", strings.Join(tags, ", "), ocrText)
	return summary, nil
}

// --- PART 1: MOBILENET TAGGING ---

func runTagging(path string) []string {
	if TagSession == nil {
		return []string{}
	}

	img, err := loadImage(path)
	if err != nil {
		return []string{}
	}

	// Resize to 224x224 (Standard for MobileNet)
	resized := resizeImage(img, 224, 224)
	inputData := imageToFloat32(resized, 224, 224)

	// Create Tensor: [1, 3, 224, 224]
	tensor, err := onnxruntime_go.NewTensor([]int64{1, 3, 224, 224}, inputData)
	if err != nil {
		return []string{}
	}

	// Run Model
	output, err := onnxruntime_go.NewTensor([]int64{1, 1000}, make([]float32, 1000))
	if err != nil {
		return []string{}
	}

	err = TagSession.Run([]*onnxruntime_go.Tensor[float32]{tensor}, []*onnxruntime_go.Tensor[float32]{output})
	if err != nil {
		fmt.Printf("Error running model: %v\n", err)
		return []string{}
	}

	// Get Top 5 Tags
	probs := output.GetData()
	type Item struct {
		Idx   int
		Score float32
	}
	items := make([]Item, 1000)
	for i, p := range probs {
		items[i] = Item{i, p}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })

	var result []string
	for i := 0; i < 5; i++ {
		if items[i].Score > 0.1 && items[i].Idx < len(ImageTags) {
			// Clean label (remove "n0234234 " prefix)
			lbl := ImageTags[items[i].Idx]
			parts := strings.Split(lbl, " ")
			if len(parts) > 1 {
				lbl = strings.Join(parts[1:], " ")
			}
			result = append(result, strings.TrimSpace(lbl))
		}
	}
	return result
}

// --- PART 2: WINDOWS NATIVE OCR ---

func runWindowsOCR(imagePath string) (string, error) {
	return runWindowsOCRWithTimeout(imagePath, 30*time.Second)
}

func runWindowsOCRWithTimeout(imagePath string, timeout time.Duration) (string, error) {
	// Validate input
	if imagePath == "" {
		return "", fmt.Errorf("image path is empty")
	}

	// Convert to absolute path to avoid ambiguity
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Escape single quotes for PowerShell and backslashes
	safePath := strings.ReplaceAll(absPath, "'", "''")

	// PowerShell script with improved error handling and type loading
	psScript := `
$path = '` + safePath + `'
$ErrorActionPreference = 'Stop'

try {
    # Validate file exists
    if (-not (Test-Path $path)) {
        Write-Error "File not found: $path"
        exit 1
    }

    # Load Required Assemblies
    Add-Type -AssemblyName System.Runtime.WindowsRuntime
    
    # Load WinRT Types - More explicit error handling
    $null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
    $null = [Windows.Storage.FileAccessMode, Windows.Storage, ContentType = WindowsRuntime]
    $null = [Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]
    $null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics, ContentType = WindowsRuntime]
    $null = [Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics, ContentType = WindowsRuntime]
    $null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]

    # Helper for Async Tasks
    $asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
        $_.Name -eq 'AsTask' -and 
        $_.GetParameters().Count -eq 1 -and 
        $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1'
    })[0]

    if ($asTaskGeneric -eq $null) {
        Write-Error "Failed to find AsTask method"
        exit 1
    }

    Function Await($WinRtTask, $ResultType) {
        if ($WinRtTask -eq $null) {
            throw "WinRT task is null"
        }
        $asTask = $asTaskGeneric.MakeGenericMethod($ResultType)
        $netTask = $asTask.Invoke($null, @($WinRtTask))
        $netTask.Wait(-1) | Out-Null
        return $netTask.Result
    }

    # Load File
    $file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($path)) ([Windows.Storage.StorageFile])
    if ($file -eq $null) {
        Write-Error "Failed to load file"
        exit 1
    }

    # Open Stream
    $stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    if ($stream -eq $null) {
        Write-Error "Failed to open file stream"
        exit 1
    }

    # Decode Bitmap
    $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    if ($decoder -eq $null) {
        Write-Error "Failed to decode image"
        exit 1
    }

    # Get Bitmap
    $bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
    if ($bitmap -eq $null) {
        Write-Error "Failed to create software bitmap"
        exit 1
    }

    # Create OCR Engine
    $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
    if ($engine -eq $null) {
        Write-Error "OCR engine not available for current language"
        exit 1
    }

    # Run OCR
    $result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
    if ($result -eq $null) {
        Write-Output ""
        exit 0
    }

    # Output text
    Write-Output $result.Text

} catch {
    Write-Error $_.Exception.Message
    exit 1
} finally {
    # Cleanup resources
    if ($stream -ne $null) { 
        try { $stream.Dispose() } catch { }
    }
}
`

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute via PowerShell
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", psScript)

	// Hide the PowerShell window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	out, err := cmd.CombinedOutput()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("OCR operation timed out after %v", timeout)
	}

	output := strings.TrimSpace(string(out))

	// Check exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// PowerShell reported an error - return the error message
			return "", fmt.Errorf("OCR failed: %s", output)
		}
		return "", fmt.Errorf("failed to execute PowerShell: %w (output: %s)", err, output)
	}

	return output, nil
}

// --- HELPERS ---

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func resizeImage(img image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Rect, img, img.Bounds(), draw.Over, nil)
	return dst
}

func imageToFloat32(img image.Image, w, h int) []float32 {
	bounds := img.Bounds()
	data := make([]float32, 3*w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// 0-65535 -> 0-1
			fr := float32(r) / 65535.0
			fg := float32(g) / 65535.0
			fb := float32(b) / 65535.0

			// Normalize (Mean=[0.485, 0.456, 0.406], Std=[0.229, 0.224, 0.225])
			fr = (fr - 0.485) / 0.229
			fg = (fg - 0.456) / 0.224
			fb = (fb - 0.406) / 0.225

			// CHW Layout
			data[0*w*h+y*w+x] = fr
			data[1*w*h+y*w+x] = fg
			data[2*w*h+y*w+x] = fb
		}
	}
	return data
}

func ensureModel(name, url string) {
	path := GetDataPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Downloading %s...\n", name)
		DownloadFile(url, path)
	}
}

func loadLabels() {
	path := GetDataPath("labels.txt")
	content, err := os.ReadFile(path)
	if err == nil {
		lines := strings.Split(string(content), "\n")
		ImageTags = lines
	}
}
