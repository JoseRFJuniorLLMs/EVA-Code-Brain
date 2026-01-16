package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GitService handles Git operations for code history analysis
type GitService struct {
	projectRoots map[string]string // project_name -> root_path
}

// GitCommit represents a commit in the history
type GitCommit struct {
	Hash         string
	Author       string
	Date         time.Time
	Message      string
	FilesChanged []string
}

// GitBlameResult represents blame information for a line
type GitBlameResult struct {
	Line        int
	Hash        string
	Author      string
	Date        time.Time
	CommitMsg   string
	LineContent string
}

// NewGitService creates a new Git service instance
func NewGitService() *GitService {
	return &GitService{
		projectRoots: make(map[string]string),
	}
}

// LoadProjectRoots loads project paths from database
func (gs *GitService) LoadProjectRoots(ctx context.Context) error {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT project_name, root_path 
		FROM project_metadata 
		WHERE root_path IS NOT NULL`)

	if err != nil {
		return fmt.Errorf("failed to load project roots: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var projectName, rootPath string
		if err := rows.Scan(&projectName, &rootPath); err != nil {
			continue
		}
		gs.projectRoots[projectName] = rootPath
	}

	return nil
}

// GetProjectRoot finds the git root for a file path
func (gs *GitService) GetProjectRoot(filePath string) (string, error) {
	// Try to find in loaded projects first
	for _, root := range gs.projectRoots {
		if strings.HasPrefix(filePath, root) {
			return root, nil
		}
	}

	// Fallback: execute git rev-parse
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(absPath)
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// Blame returns git blame information for a file
func (gs *GitService) Blame(filePath string, startLine, endLine int) ([]GitBlameResult, error) {
	root, err := gs.GetProjectRoot(filePath)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		return nil, err
	}

	// git blame -L start,end --porcelain file
	lineRange := fmt.Sprintf("%d,%d", startLine, endLine)
	cmd := exec.Command("git", "blame", "-L", lineRange, "--porcelain", relPath)
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git blame failed: %w", err)
	}

	return gs.parseBlameOutput(output, startLine)
}

// parseBlameOutput parses git blame --porcelain output
func (gs *GitService) parseBlameOutput(output []byte, startLine int) ([]GitBlameResult, error) {
	lines := strings.Split(string(output), "\n")
	var results []GitBlameResult
	var currentResult GitBlameResult
	lineNum := startLine

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if len(line) == 0 {
			continue
		}

		// First line of each block: hash lineNum finalLineNum numLines
		if !strings.HasPrefix(line, "\t") && len(strings.Fields(line)) >= 2 {
			fields := strings.Fields(line)
			currentResult = GitBlameResult{
				Hash: fields[0],
				Line: lineNum,
			}
			lineNum++
		} else if strings.HasPrefix(line, "author ") {
			currentResult.Author = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "author-time ") {
			timestamp := strings.TrimPrefix(line, "author-time ")
			if ts, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
				currentResult.Date = time.Unix(ts, 0)
			}
		} else if strings.HasPrefix(line, "summary ") {
			currentResult.CommitMsg = strings.TrimPrefix(line, "summary ")
		} else if strings.HasPrefix(line, "\t") {
			// Actual line content
			currentResult.LineContent = strings.TrimPrefix(line, "\t")
			results = append(results, currentResult)
		}
	}

	return results, nil
}

// Log returns commit history for a file
func (gs *GitService) Log(filePath string, limit int) ([]GitCommit, error) {
	root, err := gs.GetProjectRoot(filePath)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		return nil, err
	}

	// git log --follow --pretty=format:"%H|%an|%at|%s" -n limit -- file
	format := "%H|%an|%at|%s"
	cmd := exec.Command("git", "log", "--follow", fmt.Sprintf("--pretty=format:%s", format),
		"-n", strconv.Itoa(limit), "--", relPath)
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	return gs.parseLogOutput(output)
}

// parseLogOutput parses git log output
func (gs *GitService) parseLogOutput(output []byte) ([]GitCommit, error) {
	lines := strings.Split(string(output), "\n")
	var commits []GitCommit

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}

		timestamp, _ := strconv.ParseInt(parts[2], 10, 64)
		commit := GitCommit{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    time.Unix(timestamp, 0),
			Message: parts[3],
		}
		commits = append(commits, commit)
	}

	return commits, nil
}

// Diff returns the diff between two commits/branches for a file
func (gs *GitService) Diff(filePath, ref1, ref2 string) (string, error) {
	root, err := gs.GetProjectRoot(filePath)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", err
	}

	// git diff ref1..ref2 -- file
	diffRange := fmt.Sprintf("%s..%s", ref1, ref2)
	cmd := exec.Command("git", "diff", diffRange, "--", relPath)
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}

	return string(output), nil
}

// SearchCommits searches commit messages semantically
func (gs *GitService) SearchCommits(ctx context.Context, projectName, query string, limit int) ([]GitCommit, error) {
	root, ok := gs.projectRoots[projectName]
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectName)
	}

	// git log --all --grep="query" --pretty=format:"%H|%an|%at|%s" -n limit
	format := "%H|%an|%at|%s"
	cmd := exec.Command("git", "log", "--all", fmt.Sprintf("--grep=%s", query),
		fmt.Sprintf("--pretty=format:%s", format), "-n", strconv.Itoa(limit))
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log search failed: %w", err)
	}

	return gs.parseLogOutput(output)
}

// GetFileAtCommit returns file content at a specific commit
func (gs *GitService) GetFileAtCommit(filePath, commitHash string) (string, error) {
	root, err := gs.GetProjectRoot(filePath)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", err
	}

	// git show commit:file
	ref := fmt.Sprintf("%s:%s", commitHash, relPath)
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show failed: %w", err)
	}

	return string(output), nil
}

// GetRecentChanges returns files changed in the last N commits
func (gs *GitService) GetRecentChanges(projectName string, numCommits int) ([]GitCommit, error) {
	root, ok := gs.projectRoots[projectName]
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectName)
	}

	// git log --name-only --pretty=format:"%H|%an|%at|%s" -n numCommits
	format := "%H|%an|%at|%s"
	cmd := exec.Command("git", "log", "--name-only",
		fmt.Sprintf("--pretty=format:%s", format), "-n", strconv.Itoa(numCommits))
	cmd.Dir = root

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	return gs.parseLogWithFiles(stdout.Bytes())
}

// parseLogWithFiles parses git log output with file names
func (gs *GitService) parseLogWithFiles(output []byte) ([]GitCommit, error) {
	blocks := strings.Split(string(output), "\n\n")
	var commits []GitCommit

	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		if len(lines) < 1 {
			continue
		}

		// First line: commit info
		parts := strings.SplitN(lines[0], "|", 4)
		if len(parts) != 4 {
			continue
		}

		timestamp, _ := strconv.ParseInt(parts[2], 10, 64)
		commit := GitCommit{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    time.Unix(timestamp, 0),
			Message: parts[3],
		}

		// Remaining lines: files changed
		for i := 1; i < len(lines); i++ {
			if lines[i] != "" {
				commit.FilesChanged = append(commit.FilesChanged, lines[i])
			}
		}

		commits = append(commits, commit)
	}

	return commits, nil
}
