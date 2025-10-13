package util

import (
	"fmt"
	"io"
	"os"
)

func CopyFile(src, dst string) (int64, error) {
	// If source and destination are the same, just return the file size
	if src == dst {
		info, err := os.Stat(src)
		if err != nil {
			return 0, fmt.Errorf("failed to stat source file: %w", err)
		}
		return info.Size(), nil
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	bytesCopied, err := io.Copy(destinationFile, sourceFile)
	if err != nil {
		return 0, fmt.Errorf("failed to copy file contents: %w", err)
	}

	return bytesCopied, nil
}
