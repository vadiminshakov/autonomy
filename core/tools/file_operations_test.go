package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	srcFile, err := os.CreateTemp("", "src_file_*.txt")
	if err != nil {
		t.Fatalf("Error creating source test file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	if _, err := srcFile.WriteString("Hello, world!"); err != nil {
		t.Fatalf("Error writing to source test file: %v", err)
	}

	destPath := filepath.Join(os.TempDir(), "dest_file.txt")
	defer os.Remove(destPath)

	args := map[string]interface{}{
		"src":  srcFile.Name(),
		"dest": destPath,
	}

	_, err = CopyFile(args)
	if err != nil {
		t.Errorf("CopyFile failed: %v", err)
	}

	_, err = os.Stat(destPath)
	if os.IsNotExist(err) {
		t.Errorf("Destination file does not exist: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Errorf("Error reading destination file: %v", err)
	}

	expectedContent := "Hello, world!"
	if string(content) != expectedContent {
		t.Errorf("Unexpected content in destination file, got: %s, want: %s", content, expectedContent)
	}
}

func TestCopyFile_MissingSrc(t *testing.T) {
	args := map[string]interface{}{
		"dest": "somepath",
	}

	_, err := CopyFile(args)
	if err == nil {
		t.Errorf("Expected an error when 'src' is missing, but got none")
	}
}

func TestCopyFile_MissingDest(t *testing.T) {
	args := map[string]interface{}{
		"src": "somepath",
	}

	_, err := CopyFile(args)
	if err == nil {
		t.Errorf("Expected an error when 'dest' is missing, but got none")
	}
}

func TestMoveAndDeleteFile(t *testing.T) {
	// create temp src file
	srcFile, err := os.CreateTemp("", "move_src_*.txt")
	if err != nil {
		t.Fatalf("Failed to create src temp file: %v", err)
	}
	srcName := srcFile.Name()
	srcFile.Close()

	destPath := filepath.Join(os.TempDir(), "moved_file.txt")
	defer os.Remove(destPath)

	// move file
	_, err = MoveFile(map[string]interface{}{"src": srcName, "dest": destPath})
	if err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}

	// assert src doesn't exist, dest exists
	if _, err := os.Stat(srcName); !os.IsNotExist(err) {
		t.Errorf("Source file still exists after move")
	}
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("Destination file missing after move: %v", err)
	}

	// delete dest file
	_, err = DeleteFile(map[string]interface{}{"path": destPath})
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Errorf("File still exists after deletion")
	}
}

func TestMakeAndRemoveDir(t *testing.T) {
	dirPath := filepath.Join(os.TempDir(), "test_make_dir_nested", "inner")

	// make dir
	_, err := MakeDir(map[string]interface{}{"path": dirPath})
	if err != nil {
		t.Fatalf("MakeDir failed: %v", err)
	}

	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Fatalf("Directory was not created correctly: %v", err)
	}

	// remove recursively parent directory
	parent := filepath.Dir(dirPath)
	_, err = RemoveDir(map[string]interface{}{"path": parent, "recursive": true})
	if err != nil {
		t.Fatalf("RemoveDir failed: %v", err)
	}

	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Errorf("Directory still exists after RemoveDir")
	}
}
