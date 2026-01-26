//go:build cloudflare

// Cloudflare Workers entry point using syumai/workers
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joeblew999/deckfs/handler"
	"github.com/joeblew999/deckfs/pkg/pipeline"
	"github.com/joeblew999/deckfs/runtime"
	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare/queues"
)

func main() {
	// Initialize runtime with R2 storage
	initRuntime()

	// Register handlers
	handler.RegisterHandlers(http.DefaultServeMux)

	// Register queue consumer for reactive processing (non-blocking)
	queues.ConsumeNonBlock(consumeQueue)

	// Start worker
	workers.Serve(nil)
}

func initRuntime() {
	inputStorage, _ := runtime.NewR2Storage("DECKFS_INPUT")
	outputStorage, _ := runtime.NewR2Storage("DECKFS_OUTPUT")
	kvStore, _ := runtime.NewCloudflareKV("DECKFS_STATUS")

	runtime.SetRuntime(&runtime.Runtime{
		InputStorage:  inputStorage,
		OutputStorage: outputStorage,
		KV:            kvStore,
	})
}

// consumeQueue handles R2 event notifications from the queue
func consumeQueue(batch *queues.MessageBatch) error {
	for _, msg := range batch.Messages {
		body, err := msg.BytesBody()
		if err != nil {
			msg.Retry()
			continue
		}

		var event struct {
			Action string `json:"action"`
			Object struct {
				Key string `json:"key"`
			} `json:"object"`
		}
		if err := json.Unmarshal(body, &event); err != nil {
			msg.Retry()
			continue
		}

		if !strings.HasSuffix(event.Object.Key, ".dsh") {
			msg.Ack()
			continue
		}

		key := event.Object.Key

		// Update status: processing
		setStatus(key, "processing", "")

		// Get source from R2
		reader, err := runtime.Input().Get(nil, key)
		if err != nil {
			setStatus(key, "error", "failed to read source")
			msg.Retry()
			continue
		}
		source, _ := io.ReadAll(reader)
		reader.Close()

		// Check if source has imports and expand them
		processSource := source
		if pipeline.HasImports(source) {
			// Create import resolver with R2 input storage
			resolver := pipeline.NewImportResolver(
				pipeline.StorageLoader(runtime.Input()),
				"", // R2 keys are already absolute-like
			)

			// Expand imports
			ctx := context.Background()
			expanded, err := resolver.Expand(ctx, source, key)
			if err != nil {
				setStatus(key, "error", "import resolution failed: "+err.Error())
				msg.Ack() // Don't retry import errors
				continue
			}
			processSource = expanded
		}

		// Process
		p := pipeline.NewWASMPipeline()
		result, err := p.Process(context.Background(), processSource, pipeline.FormatSVG)
		if err != nil {
			setStatus(key, "error", err.Error())
			msg.Ack() // Don't retry bad source
			continue
		}

		// Store outputs
		baseName := strings.TrimSuffix(key, ".dsh")
		for i, slide := range result.Slides {
			slideKey := baseName + "/slide-" + padInt(i+1, 4) + ".svg"
			runtime.Output().Put(nil, slideKey, slide, "image/svg+xml")
		}

		// Store manifest
		manifest := map[string]any{
			"sourceKey":   key,
			"processedAt": time.Now().UTC().Format(time.RFC3339),
			"title":       result.Title,
			"slideCount":  result.SlideCount,
		}
		manifestJSON, _ := json.Marshal(manifest)
		runtime.Output().Put(nil, baseName+"/manifest.json", manifestJSON, "application/json")

		// Update status: complete
		setStatus(key, "complete", "")

		msg.Ack()
	}
	return nil
}

func padInt(n, width int) string {
	s := ""
	for i := 0; i < width; i++ {
		s = string('0'+(n%10)) + s
		n /= 10
	}
	return s
}

func setStatus(key, status, errMsg string) {
	data := map[string]any{
		"status":    status,
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	if errMsg != "" {
		data["error"] = errMsg
	}
	jsonData, _ := json.Marshal(data)
	runtime.KV().Put(nil, "status:"+key, jsonData)
}
