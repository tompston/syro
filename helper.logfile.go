package syro

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"
)

// LogBuffer is a minimal, thread-safe log buffer. It accumulates lines in
// memory until ToFile is called. Useful for accumumating logs of a
// single prolonged operation, that you don't want to log to
// disk right away or write to db each time
type LogBuffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func NewLogBuffer() *LogBuffer { return &LogBuffer{} }

// Logf appends a formatted line to the buffer (thread-safe).
// Always ensures a trailing newline.
func (lb *LogBuffer) Logf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	lb.mu.Lock()
	lb.sb.WriteString(s)
	if !strings.HasSuffix(s, "\n") {
		lb.sb.WriteByte('\n')
	}
	lb.mu.Unlock()
}

func (lb *LogBuffer) Reset() {
	lb.mu.Lock()
	lb.sb.Reset()
	lb.mu.Unlock()
}

// ToFile returns the accumulated log contents as []byte,
// and clears the in-memory buffer (thread-safe).
func (lb *LogBuffer) ToFile() []byte {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	data := []byte(lb.sb.String())
	lb.sb.Reset() // free memory
	return data
}

func (lb *LogBuffer) ToZip(filename string) ([]byte, error) {
	fileBytes := lb.ToFile()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	h := &zip.FileHeader{
		Name:     filename,
		Method:   zip.Deflate,
		Modified: time.Now().UTC(),
	}

	w, err := zw.CreateHeader(h)
	if err != nil {
		return nil, fmt.Errorf("failed to create zip entry: %v", err)
	}

	if _, err := w.Write(fileBytes); err != nil {
		return nil, fmt.Errorf("failed to write log bytes into zip: %v", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip: %v", err)
	}

	return buf.Bytes(), nil
}
