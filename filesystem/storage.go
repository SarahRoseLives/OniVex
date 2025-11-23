package filesystem

import (
	"net/http"
	"os"
)

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