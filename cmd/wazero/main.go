//go:build !js

// Wazero host - loads deckfs WASM from R2 and runs it
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	var (
		wasmURL    = flag.String("wasm", "", "URL to WASM file (R2 public URL)")
		wasmFile   = flag.String("wasm-file", "", "Local WASM file path")
		addr       = flag.String("addr", ":8080", "Listen address")
		r2Endpoint = flag.String("r2-endpoint", "", "R2 S3 endpoint")
		r2Bucket   = flag.String("r2-bucket", "", "R2 bucket name")
		r2Key      = flag.String("r2-key", "", "R2 access key")
		r2Secret   = flag.String("r2-secret", "", "R2 secret key")
	)
	flag.Parse()

	ctx := context.Background()

	// Load WASM from R2, local file, or URL
	var wasmBytes []byte
	var err error

	switch {
	case *wasmFile != "":
		log.Printf("Loading WASM from file: %s", *wasmFile)
		wasmBytes, err = os.ReadFile(*wasmFile)
	case *wasmURL != "":
		log.Printf("Loading WASM from URL: %s", *wasmURL)
		wasmBytes, err = fetchURL(*wasmURL)
	default:
		log.Fatal("Either -wasm or -wasm-file must be specified")
	}

	if err != nil {
		log.Fatalf("Failed to load WASM: %v", err)
	}

	log.Printf("WASM loaded: %d bytes", len(wasmBytes))

	// Create wazero runtime
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// TODO: Add host functions for R2 access
	// The WASM module needs to call back to the host for storage operations
	// This requires implementing host functions that the WASM can import

	// For now, we'll run a simple HTTP proxy that handles storage on the host side
	// and forwards processing requests to the WASM module

	// Compile the module
	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		log.Fatalf("Failed to compile WASM: %v", err)
	}

	log.Printf("WASM compiled successfully")

	// Create HTTP server that uses the WASM for processing
	server := &WazeroServer{
		runtime:    r,
		compiled:   compiled,
		r2Endpoint: *r2Endpoint,
		r2Bucket:   *r2Bucket,
		r2Key:      *r2Key,
		r2Secret:   *r2Secret,
	}

	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, server); err != nil {
		log.Fatal(err)
	}
}

type WazeroServer struct {
	runtime    wazero.Runtime
	compiled   wazero.CompiledModule
	r2Endpoint string
	r2Bucket   string
	r2Key      string
	r2Secret   string
}

func (s *WazeroServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// For /process endpoint, run WASM
	// For storage endpoints, use R2 HTTP API directly

	switch {
	case r.URL.Path == "/health":
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","runtime":"wazero"}`))

	case r.URL.Path == "/process" && r.Method == "POST":
		s.handleProcess(w, r)

	default:
		// Proxy to R2 for storage operations
		s.proxyToR2(w, r)
	}
}

func (s *WazeroServer) handleProcess(w http.ResponseWriter, r *http.Request) {
	// Read input
	source, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Create stdout writer to capture output
	stdout := NewBytesWriter()

	// Create a new module instance for this request
	// Pass source via stdin, get result via stdout
	config := wazero.NewModuleConfig().
		WithStdin(NewBytesReader(source)).
		WithStdout(stdout).
		WithStderr(os.Stderr).
		WithArgs("deckfs", "process")

	mod, err := s.runtime.InstantiateModule(ctx, s.compiled, config)
	if err != nil {
		http.Error(w, fmt.Sprintf("WASM execution failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer mod.Close(ctx)

	// Write output from stdout writer
	w.Header().Set("Content-Type", "application/json")
	w.Write(stdout.Bytes())
}

func (s *WazeroServer) proxyToR2(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement R2 proxy
	http.Error(w, "R2 proxy not implemented", http.StatusNotImplemented)
}

func fetchURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// BytesReader wraps []byte for stdin
type bytesReader struct {
	data []byte
	pos  int
}

func NewBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// BytesWriter captures stdout
type bytesWriter struct {
	data []byte
}

func NewBytesWriter() *bytesWriter {
	return &bytesWriter{}
}

func (w *bytesWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *bytesWriter) Bytes() []byte {
	return w.data
}
