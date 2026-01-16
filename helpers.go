package main

// extractFilesFromResults extracts unique file paths from search results
func extractFilesFromResults(results []SearchResult) []string {
	filesMap := make(map[string]bool)
	for _, r := range results {
		if r.FilePath != "" {
			filesMap[r.FilePath] = true
		}
	}

	var files []string
	for file := range filesMap {
		files = append(files, file)
	}
	return files
}
