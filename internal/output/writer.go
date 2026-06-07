package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Writer is the interface for output writers
type Writer interface {
	Write(record map[string]interface{}) error
	Close() error
}

// NewWriter creates a writer based on the format string
func NewWriter(format, path string) (Writer, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	switch format {
	case "jsonl", "":
		return NewJSONLWriter(path)
	case "csv":
		return NewCSVWriter(path)
	default:
		return nil, fmt.Errorf("unknown output format: %s", format)
	}
}

// JSONLWriter writes records as JSON Lines (one JSON object per line)
type JSONLWriter struct {
	mu   sync.Mutex
	file *os.File
}

// NewJSONLWriter creates a JSONL file writer (appends if file exists)
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening jsonl file: %w", err)
	}
	return &JSONLWriter{file: f}, nil
}

// Write serializes the record as a JSON line
func (w *JSONLWriter) Write(record map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshaling record: %w", err)
	}
	_, err = fmt.Fprintf(w.file, "%s\n", data)
	return err
}

// Close flushes and closes the file
func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// CSVWriter writes records as CSV rows
type CSVWriter struct {
	mu      sync.Mutex
	file    *os.File
	writer  *csv.Writer
	headers []string
	wrote   bool
}

// NewCSVWriter creates a CSV file writer
func NewCSVWriter(path string) (*CSVWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening csv file: %w", err)
	}
	return &CSVWriter{
		file:   f,
		writer: csv.NewWriter(f),
	}, nil
}

// Write writes a record to CSV (auto-detects headers from first record)
func (w *CSVWriter) Write(record map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write header from first record
	if !w.wrote {
		w.headers = make([]string, 0, len(record))
		for k := range record {
			w.headers = append(w.headers, k)
		}
		if err := w.writer.Write(w.headers); err != nil {
			return fmt.Errorf("writing csv header: %w", err)
		}
		w.wrote = true
	}

	// Build row in header order
	row := make([]string, len(w.headers))
	for i, h := range w.headers {
		v := record[h]
		if v == nil {
			row[i] = ""
		} else {
			switch val := v.(type) {
			case string:
				row[i] = val
			default:
				b, _ := json.Marshal(val)
				row[i] = string(b)
			}
		}
	}

	if err := w.writer.Write(row); err != nil {
		return fmt.Errorf("writing csv row: %w", err)
	}
	w.writer.Flush()
	return w.writer.Error()
}

// Close flushes and closes the file
func (w *CSVWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writer.Flush()
	return w.file.Close()
}
