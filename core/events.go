package core

import (
	"context"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var globalCtx context.Context

func SetContext(ctx context.Context) {
	globalCtx = ctx
}

// EmitProgress: type = "indexing" or "download"
func EmitProgress(taskType string, message string, percent int) {
	if globalCtx != nil {
		wruntime.EventsEmit(globalCtx, "progress:update", map[string]interface{}{
			"type":    taskType,
			"message": message,
			"percent": percent,
		})
	}
}
