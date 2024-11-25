package writer

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

type FileWriter struct {
	writeQueue chan writeOperation
	outputDir  string
}

type writeOperation struct {
	url  string
	data string
}

func NewFileWriter(outputDir string) *FileWriter {
	fw := &FileWriter{
		writeQueue: make(chan writeOperation, 1000),
		outputDir:  outputDir,
	}
	go fw.writeWorker()
	return fw
}

func (fw *FileWriter) Write(urlString string, data string) error {
	filename := filepath.Join(fw.outputDir, fmt.Sprintf("%s.md", url.QueryEscape(urlString)))

	if err := os.MkdirAll(fw.outputDir, 0755); err != nil {
		return err
	}

	fw.writeQueue <- writeOperation{
		url:  filename,
		data: data,
	}
	return nil

}

func (fw *FileWriter) writeWorker() {
	for op := range fw.writeQueue {
		file, err := os.OpenFile(op.url, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}

		file.WriteString(op.data + "\n")
		file.Close()
	}
}
