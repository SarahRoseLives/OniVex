package filesystem

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FileMeta represents a file available for download
type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Path string `json:"path"` // Relative path for the URL
}

// EnsureDirectories creates uploads/downloads folders if they don't exist
func EnsureDirectories() {
	dirs := []string{"uploads", "downloads"}
	for _, d := range dirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			os.Mkdir(d, 0755)
		}
	}
}

// GetFileHandler returns an HTTP handler that serves the uploads folder
func GetFileHandler() http.Handler {
	return http.FileServer(http.Dir("./uploads"))
}

// GetFileList scans the uploads folder and returns JSON-ready metadata
func GetFileList() ([]FileMeta, error) {
	return scanDirectory("uploads")
}

// GetDownloadsList scans the downloads folder for the local library
func GetDownloadsList() ([]FileMeta, error) {
	return scanDirectory("downloads")
}

// Helper function to scan a specific directory
func scanDirectory(dirName string) ([]FileMeta, error) {
	var files []FileMeta

	// Ensure dir exists before walking
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		return files, nil
	}

	err := filepath.Walk(dirName, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Create a relative web path (e.g., uploads/foo.txt -> /foo.txt)
			relPath := strings.TrimPrefix(path, dirName)
			// Ensure forward slashes for URLs even on Windows
			relPath = filepath.ToSlash(relPath)

			files = append(files, FileMeta{
				Name: info.Name(),
				Size: info.Size(),
				Path: relPath,
			})
		}
		return nil
	})

	return files, err
}

// SearchLocal filters the file list by a query string (case-insensitive)
func SearchLocal(query string) []FileMeta {
	allFiles, _ := GetFileList()
	if query == "" {
		return allFiles
	}

	var results []FileMeta
	query = strings.ToLower(query)

	for _, f := range allFiles {
		if strings.Contains(strings.ToLower(f.Name), query) {
			results = append(results, f)
		}
	}
	return results
}