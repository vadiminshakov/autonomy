package tools

import (
	"fmt"
	"io"
	"os"
)

// Register the file operations tool
func init() {
	Register("copy_file", CopyFile)
	Register("move_file", MoveFile)
	Register("delete_file", DeleteFile)
	Register("make_dir", MakeDir)
	Register("remove_dir", RemoveDir)
}

// CopyFile copies a file from source path to destination path
func CopyFile(args map[string]interface{}) (string, error) {
	srcPath, okSrc := args["src"].(string)
	destPath, okDest := args["dest"].(string)

	if !okSrc || !okDest {
		return "", fmt.Errorf("both 'src' and 'dest' parameters must be provided")
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, srcFile); err != nil {
		return "", fmt.Errorf("failed to copy file: %v", err)
	}

	getTaskState().RecordFileCreated(destPath)

	return "File copied successfully", nil
}

// MoveFile moves/renames a file from src to dest.
func MoveFile(args map[string]interface{}) (string, error) {
	srcPath, okSrc := args["src"].(string)
	destPath, okDest := args["dest"].(string)

	if !okSrc || !okDest {
		return "", fmt.Errorf("both 'src' and 'dest' parameters must be provided")
	}

	if err := os.Rename(srcPath, destPath); err != nil {
		return "", fmt.Errorf("failed to move file: %v", err)
	}

	getTaskState().RecordFileModified(destPath)

	return "File moved successfully", nil
}

// DeleteFile removes a file specified by 'path'.
func DeleteFile(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("parameter 'path' is required for delete_file")
	}

	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("failed to delete file: %v", err)
	}

	return "File deleted successfully", nil
}

// MakeDir creates a directory (and parents) at the specified path.
func MakeDir(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("parameter 'path' is required for make_dir")
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	return "Directory created successfully", nil
}

// RemoveDir removes a directory. If 'recursive' is true, removes it recursively.
func RemoveDir(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("parameter 'path' is required for remove_dir")
	}

	recursive := false
	if v, ok := args["recursive"].(bool); ok {
		recursive = v
	} else if v, ok := args["recursive"].(string); ok {
		recursive = (v == "true" || v == "1")
	}

	var err error
	if recursive {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		return "", fmt.Errorf("failed to remove directory: %v", err)
	}

	return "Directory removed successfully", nil
}
