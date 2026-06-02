// Package debug provides file-based request/response dump logging for debugging.
package debug

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// DumpDir is the directory where debug dumps are written.
const DumpDir = "debug-dumps"

var enabled bool

// SetEnabled enables or disables debug dump logging.
// Call this before processing requests (e.g., from a CLI flag).
func SetEnabled(v bool) {
	enabled = v
}

// IsEnabled returns whether debug dumping is enabled.
func IsEnabled() bool {
	return enabled
}

// DumpRequest writes a request body to a timestamped file.
// Returns the file path, or empty string if dumping is disabled.
func DumpRequest(model, endpoint string, body []byte) string {
	if !enabled {
		return ""
	}
	return dump("request", model, endpoint, body)
}

// DumpResponse writes a response body to a timestamped file.
// Returns the file path, or empty string if dumping is disabled.
func DumpResponse(model, endpoint string, body []byte) string {
	if !enabled {
		return ""
	}
	return dump("response", model, endpoint, body)
}

// DumpStreamLine writes a raw SSE data line to a timestamped stream file.
// Returns the file path (same file for all lines in a session), or empty string if disabled.
func DumpStreamLine(model, filePath string, line []byte) {
	if !enabled {
		return
	}
	appendLine(filePath, line)
}

// CreateStreamFile creates a new stream dump file and returns its path.
func CreateStreamFile(model string) string {
	if !enabled {
		return ""
	}
	ts := time.Now().Format("20060102-150405.000")
	filename := fmt.Sprintf("stream-%s-%s.txt", model, ts)
	ensureDir()
	path := filepath.Join(DumpDir, filename)
	// Create empty file
	_ = os.WriteFile(path, nil, 0644)
	return path
}

// CreateDownstreamFile creates a new downstream dump file and returns its path.
func CreateDownstreamFile(model string) string {
	if !enabled {
		return ""
	}
	ts := time.Now().Format("20060102-150405.000")
	filename := fmt.Sprintf("downstream-%s-%s.txt", model, ts)
	ensureDir()
	path := filepath.Join(DumpDir, filename)
	// Create empty file
	_ = os.WriteFile(path, nil, 0644)
	return path
}

func dump(kind, model, endpoint string, body []byte) string {
	ts := time.Now().Format("20060102-150405.000")
	filename := fmt.Sprintf("%s-%s-%s-%s.json", kind, model, endpoint, ts)
	ensureDir()
	path := filepath.Join(DumpDir, filename)
	if err := os.WriteFile(path, body, 0644); err != nil {
		return ""
	}
	return path
}

func appendLine(filePath string, line []byte) {
	if filePath == "" {
		return
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(line)
	_, _ = f.Write([]byte("\n"))
}

func ensureDir() {
	_ = os.MkdirAll(DumpDir, 0755)
}

// DumpReader wraps an io.Reader and writes all data to a file as it's read.
// This is used to capture streaming response bodies for debugging.
func DumpReader(r io.Reader, filePath string) io.Reader {
	if !enabled || filePath == "" {
		return r
	}
	return &dumpReader{
		r:    r,
		path: filePath,
	}
}

type dumpReader struct {
	r    io.Reader
	path string
}

func (d *dumpReader) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if n > 0 {
		f, ferr := os.OpenFile(d.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if ferr == nil {
			_, _ = f.Write(p[:n])
			f.Close()
		}
	}
	return n, err
}

// DumpWriter wraps an io.Writer and writes all data to a file as it's written.
// This is used to capture what go-proxy sends downstream to the client.
func DumpWriter(w io.Writer, filePath string) io.Writer {
	if !enabled || filePath == "" {
		return w
	}
	return &dumpWriter{
		w:    w,
		path: filePath,
	}
}

type dumpWriter struct {
	w    io.Writer
	path string
}

func (d *dumpWriter) Write(p []byte) (int, error) {
	n, err := d.w.Write(p)
	if n > 0 {
		f, ferr := os.OpenFile(d.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if ferr == nil {
			_, _ = f.Write(p[:n])
			f.Close()
		}
	}
	return n, err
}

func (d *dumpWriter) Flush() {
	if flusher, ok := d.w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
}
