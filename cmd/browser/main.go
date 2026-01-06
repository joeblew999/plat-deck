//go:build js && wasm && !cloudflare

// Browser/standard WASM entry point
// Exposes functions to JavaScript and uses fetch() for R2 access
package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"github.com/joeblew999/deckfs/internal/processor"
	"github.com/joeblew999/deckfs/runtime"
)

func main() {
	// Export functions to JavaScript
	js.Global().Set("deckfs", js.ValueOf(map[string]any{
		"version":   js.FuncOf(version),
		"process":   js.FuncOf(process),
		"configure": js.FuncOf(configure),
	}))

	// Keep alive
	select {}
}

// version returns the module version
func version(this js.Value, args []js.Value) any {
	return "deckfs-wasm v0.1.0 (browser)"
}

// configure sets up R2 storage access
// Usage: deckfs.configure({inputURL: "https://...", outputURL: "https://..."})
func configure(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("missing config argument")
	}

	config := args[0]

	inputURL := config.Get("inputURL").String()
	outputURL := config.Get("outputURL").String()

	var inputStorage, outputStorage runtime.Storage

	if inputURL != "" {
		inputStorage = runtime.NewPublicR2Storage(inputURL)
	}
	if outputURL != "" {
		outputStorage = runtime.NewPublicR2Storage(outputURL)
	}

	runtime.SetRuntime(&runtime.Runtime{
		InputStorage:  inputStorage,
		OutputStorage: outputStorage,
	})

	return successResult(map[string]any{
		"configured": true,
	})
}

// process converts decksh source to SVG
// Usage: deckfs.process(source, config?) -> JSON result
func process(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("missing source argument")
	}

	source := args[0].String()

	cfg := processor.DefaultConfig()

	// Parse optional config
	if len(args) >= 2 && !args[1].IsUndefined() && !args[1].IsNull() {
		jsConfig := args[1]
		if w := jsConfig.Get("width"); !w.IsUndefined() {
			cfg.Width = w.Int()
		}
		if h := jsConfig.Get("height"); !h.IsUndefined() {
			cfg.Height = h.Int()
		}
	}

	result, err := processor.ProcessDeckSH([]byte(source), cfg)
	if err != nil {
		return errorResult(err.Error())
	}

	slides := make([]string, len(result.Slides))
	for i, s := range result.Slides {
		slides[i] = string(s)
	}

	return successResult(map[string]any{
		"title":      result.Title,
		"slideCount": result.SlideCount,
		"slides":     slides,
	})
}

// processFromR2 fetches source from R2, processes it, stores results
// Usage: deckfs.processFromR2(key) -> Promise
func processFromR2(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("missing key argument")
	}

	key := args[0].String()

	// Return a Promise
	handler := js.FuncOf(func(this js.Value, promiseArgs []js.Value) any {
		resolve := promiseArgs[0]
		reject := promiseArgs[1]

		go func() {
			ctx := context.Background()

			// Fetch from R2
			reader, err := runtime.Input().Get(ctx, key)
			if err != nil {
				reject.Invoke(err.Error())
				return
			}

			source := make([]byte, 0)
			buf := make([]byte, 4096)
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					source = append(source, buf[:n]...)
				}
				if err != nil {
					break
				}
			}
			reader.Close()

			// Process
			cfg := processor.DefaultConfig()
			result, err := processor.ProcessDeckSH(source, cfg)
			if err != nil {
				reject.Invoke(err.Error())
				return
			}

			resolve.Invoke(successResult(map[string]any{
				"title":      result.Title,
				"slideCount": result.SlideCount,
			}))
		}()

		return nil
	})

	return js.Global().Get("Promise").New(handler)
}

func successResult(data map[string]any) string {
	data["success"] = true
	b, _ := json.Marshal(data)
	return string(b)
}

func errorResult(msg string) string {
	b, _ := json.Marshal(map[string]any{
		"success": false,
		"error":   msg,
	})
	return string(b)
}
