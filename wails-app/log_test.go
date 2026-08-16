package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLogFileUsesUTF8BOMAndKeepsVietnamese(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.log")
	if err := writeLogFile(path, "Tập 01: hoàn tất\nTập 02: lỗi"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("log file must start with a UTF-8 BOM")
	}
	if !bytes.Contains(data, []byte("Tập 01: hoàn tất")) {
		t.Fatal("Vietnamese log content was not preserved")
	}
}
