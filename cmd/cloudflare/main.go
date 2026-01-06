//go:build cloudflare

// Cloudflare Workers entry point using syumai/workers
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joeblew999/deckfs/handler"
	"github.com/joeblew999/deckfs/internal/processor"
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

	runtime.SetRuntime(&runtime.Runtime{
		InputStorage:  inputStorage,
		OutputStorage: outputStorage,
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

		// Get source from R2
		reader, err := runtime.Input().Get(nil, event.Object.Key)
		if err != nil {
			msg.Retry()
			continue
		}
		source, _ := io.ReadAll(reader)
		reader.Close()

		// Process
		cfg := processor.DefaultConfig()
		result, err := processor.ProcessDeckSH(source, cfg)
		if err != nil {
			msg.Ack() // Don't retry bad source
			continue
		}

		// Store outputs
		baseName := strings.TrimSuffix(event.Object.Key, ".dsh")
		for i, slide := range result.Slides {
			slideKey := baseName + "/slide-" + padInt(i+1, 4) + ".svg"
			runtime.Output().Put(nil, slideKey, slide, "image/svg+xml")
		}

		// Store manifest
		manifest := map[string]any{
			"sourceKey":   event.Object.Key,
			"processedAt": time.Now().UTC().Format(time.RFC3339),
			"title":       result.Title,
			"slideCount":  result.SlideCount,
		}
		manifestJSON, _ := json.Marshal(manifest)
		runtime.Output().Put(nil, baseName+"/manifest.json", manifestJSON, "application/json")

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
