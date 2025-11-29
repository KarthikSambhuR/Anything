package core

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/fcjr/geticon"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	user32             = syscall.NewLazyDLL("user32.dll")
	gdi32              = syscall.NewLazyDLL("gdi32.dll")
	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procDestroyIcon    = user32.NewProc("DestroyIcon")
	procGetIconInfo    = user32.NewProc("GetIconInfo")
	procGetDIBits      = gdi32.NewProc("GetDIBits")
	procDeleteObject   = gdi32.NewProc("DeleteObject")
)

const (
	SHGFI_ICON              = 0x000000100
	SHGFI_LARGEICON         = 0x000000000
	SHGFI_SMALLICON         = 0x000000001
	SHGFI_USEFILEATTRIBUTES = 0x000000010
	FILE_ATTRIBUTE_NORMAL   = 0x00000080
)

type SHFILEINFO struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

type ICONINFO struct {
	fIcon    uint32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type BITMAP struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type BITMAPINFOHEADER struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// Extract icon using Windows Shell32 API via syscalls
func getIconFromShell(path string) (image.Image, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return nil, err
	}

	var shfi SHFILEINFO

	// Get the icon handle using SHGetFileInfoW
	ret, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&shfi)),
		unsafe.Sizeof(shfi),
		SHGFI_ICON|SHGFI_LARGEICON,
	)

	if ret == 0 || shfi.hIcon == 0 {
		return nil, syscall.EINVAL
	}
	defer procDestroyIcon.Call(shfi.hIcon)

	// Get icon info to access the bitmaps
	var iconInfo ICONINFO
	ret, _, _ = procGetIconInfo.Call(shfi.hIcon, uintptr(unsafe.Pointer(&iconInfo)))
	if ret == 0 {
		return nil, syscall.EINVAL
	}
	defer procDeleteObject.Call(iconInfo.hbmColor)
	defer procDeleteObject.Call(iconInfo.hbmMask)

	// Get raw bitmap info
	var bm BITMAP
	gdi32.NewProc("GetObjectW").Call(
		iconInfo.hbmColor,
		unsafe.Sizeof(bm),
		uintptr(unsafe.Pointer(&bm)),
	)

	width := int(bm.bmWidth)
	height := int(bm.bmHeight)

	// Prepare BITMAPINFO structure for GetDIBits
	bi := BITMAPINFOHEADER{
		biSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		biWidth:       int32(width),
		biHeight:      -int32(height), // Negative height requests a top-down DIB
		biPlanes:      1,
		biBitCount:    32,
		biCompression: 0, // BI_RGB
	}

	bufferSize := width * height * 4
	buffer := make([]byte, bufferSize)

	hdc, _, _ := user32.NewProc("GetDC").Call(0)
	defer user32.NewProc("ReleaseDC").Call(0, hdc)

	// Extract pixel data into buffer
	ret, _, _ = procGetDIBits.Call(
		hdc,
		iconInfo.hbmColor,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&bi)),
		0, // DIB_RGB_COLORS
	)

	if ret == 0 {
		return nil, syscall.EINVAL
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Manually convert BGRA (Windows default) to RGBA (Go image)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			b := buffer[offset]
			g := buffer[offset+1]
			r := buffer[offset+2]
			a := buffer[offset+3]

			imgOffset := img.PixOffset(x, y)
			img.Pix[imgOffset] = r
			img.Pix[imgOffset+1] = g
			img.Pix[imgOffset+2] = b
			img.Pix[imgOffset+3] = a
		}
	}

	return img, nil
}

func isImageFile(ext string) bool {
	ext = strings.ToLower(ext)
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tiff", ".tif", ".ico"}
	for _, imgExt := range imageExts {
		if ext == imgExt {
			return true
		}
	}
	return false
}

func isVideoFile(ext string) bool {
	ext = strings.ToLower(ext)
	videoExts := []string{".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg"}
	for _, vidExt := range videoExts {
		if ext == vidExt {
			return true
		}
	}
	return false
}

func GetImageThumbnail(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	maxSize := 256
	if width <= maxSize && height <= maxSize {
		return img, nil
	}

	// Calculate aspect ratio
	var newWidth, newHeight int
	if width > height {
		newWidth = maxSize
		newHeight = (height * maxSize) / width
	} else {
		newHeight = maxSize
		newWidth = (width * maxSize) / height
	}

	// Nearest neighbor resizing (fastest for thumbnails)
	thumbnail := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := (x * width) / newWidth
			srcY := (y * height) / newHeight
			thumbnail.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return thumbnail, nil
}

// GetAppIconBase64 retrieves specific icons for files (Exe, Lnk, Images)
func GetAppIconBase64(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	var img image.Image

	if isImageFile(ext) {
		// Attempt thumbnail generation first
		img, err = GetImageThumbnail(absPath)
		if err != nil {
			// Fallback to system icon
			img, err = getIconFromShell(absPath)
			if err != nil {
				return ""
			}
		}
	} else if isVideoFile(ext) {
		// Windows Shell32 extracts video thumbnails automatically
		img, err = getIconFromShell(absPath)
		if err != nil {
			return ""
		}
	} else if ext == ".lnk" || ext == ".exe" {
		// Shell32 is more reliable for resolving Windows Shortcuts and Exe resources
		img, err = getIconFromShell(absPath)
		if err != nil {
			img, err = geticon.FromPath(absPath)
			if err != nil {
				return ""
			}
		}
	} else {
		// For standard documents, try geticon lib first, then Shell32 fallback
		img, err = geticon.FromPath(absPath)
		if err != nil {
			img, err = getIconFromShell(absPath)
			if err != nil {
				return ""
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// GetExtensionIconBase64 retrieves the generic system icon for a file extension
func GetExtensionIconBase64(ext string) string {
	if ext == "" {
		return ""
	}

	// Create a dummy filename; the file does not need to exist
	dummyPath := "dummy" + ext

	pathPtr, err := syscall.UTF16PtrFromString(dummyPath)
	if err != nil {
		return ""
	}

	var shfi SHFILEINFO

	// Use SHGFI_USEFILEATTRIBUTES to look up the icon by extension/type only
	ret, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		FILE_ATTRIBUTE_NORMAL,
		uintptr(unsafe.Pointer(&shfi)),
		unsafe.Sizeof(shfi),
		SHGFI_ICON|SHGFI_LARGEICON|SHGFI_USEFILEATTRIBUTES,
	)

	if ret == 0 || shfi.hIcon == 0 {
		return ""
	}
	defer procDestroyIcon.Call(shfi.hIcon)

	var iconInfo ICONINFO
	ret, _, _ = procGetIconInfo.Call(shfi.hIcon, uintptr(unsafe.Pointer(&iconInfo)))
	if ret == 0 {
		return ""
	}
	defer procDeleteObject.Call(iconInfo.hbmColor)
	defer procDeleteObject.Call(iconInfo.hbmMask)

	var bm BITMAP
	gdi32.NewProc("GetObjectW").Call(
		iconInfo.hbmColor,
		unsafe.Sizeof(bm),
		uintptr(unsafe.Pointer(&bm)),
	)

	width := int(bm.bmWidth)
	height := int(bm.bmHeight)

	bi := BITMAPINFOHEADER{
		biSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
		biWidth:       int32(width),
		biHeight:      -int32(height),
		biPlanes:      1,
		biBitCount:    32,
		biCompression: 0,
	}

	bufferSize := width * height * 4
	buffer := make([]byte, bufferSize)

	hdc, _, _ := user32.NewProc("GetDC").Call(0)
	defer user32.NewProc("ReleaseDC").Call(0, hdc)

	ret, _, _ = procGetDIBits.Call(
		hdc,
		iconInfo.hbmColor,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&bi)),
		0,
	)

	if ret == 0 {
		return ""
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Convert BGRA to RGBA
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			b := buffer[offset]
			g := buffer[offset+1]
			r := buffer[offset+2]
			a := buffer[offset+3]

			imgOffset := img.PixOffset(x, y)
			img.Pix[imgOffset] = r
			img.Pix[imgOffset+1] = g
			img.Pix[imgOffset+2] = b
			img.Pix[imgOffset+3] = a
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
