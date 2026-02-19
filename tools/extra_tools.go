package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/package-register/trpc-agent-go-extensions/pathutil"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// resolvePath validates a path to prevent directory traversal attacks,
// and resolves a relative path within the base directory.
func resolvePath(baseDir, relativePath string) (string, error) {
	return pathutil.ResolveSafePath(baseDir, relativePath)
}

// --- delete_file ---

type deleteFileRequest struct {
	Path string `json:"path" jsonschema:"description=The relative filepath from the base directory to delete."`
}

type deleteFileResponse struct {
	BaseDirectory string `json:"base_directory"`
	Path          string `json:"path"`
	Message       string `json:"message"`
}

func deleteFileHandler(baseDir string) func(context.Context, *deleteFileRequest) (*deleteFileResponse, error) {
	return func(_ context.Context, req *deleteFileRequest) (*deleteFileResponse, error) {
		rsp := &deleteFileResponse{BaseDirectory: baseDir, Path: req.Path}
		filePath, err := resolvePath(baseDir, req.Path)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: file not found: %v", err)
			return rsp, fmt.Errorf("file not found '%s': %w", req.Path, err)
		}
		if stat.IsDir() {
			rsp.Message = fmt.Sprintf("Error: '%s' is a directory, use delete_directory instead", req.Path)
			return rsp, fmt.Errorf("'%s' is a directory, use delete_directory instead", req.Path)
		}
		if err := os.Remove(filePath); err != nil {
			rsp.Message = fmt.Sprintf("Error: cannot delete file: %v", err)
			return rsp, fmt.Errorf("deleting file '%s': %w", req.Path, err)
		}
		rsp.Message = fmt.Sprintf("Successfully deleted file: %s", req.Path)
		return rsp, nil
	}
}

func deleteFileTool(baseDir string) tool.Tool {
	return function.NewFunctionTool(
		deleteFileHandler(baseDir),
		function.WithName("delete_file"),
		function.WithDescription("Deletes a single file. "+
			"The 'path' parameter is a relative path from the base directory (e.g., 'subdir/file.txt'). "+
			"Returns an error if the path points to a directory; use delete_directory for directories."),
	)
}

// --- delete_directory ---

type deleteDirectoryRequest struct {
	Path      string `json:"path" jsonschema:"description=The relative path of the directory to delete."`
	Recursive bool   `json:"recursive" jsonschema:"description=If true, delete directory and all contents recursively. If false, only delete if empty."`
}

type deleteDirectoryResponse struct {
	BaseDirectory string `json:"base_directory"`
	Path          string `json:"path"`
	Message       string `json:"message"`
}

func deleteDirectoryHandler(baseDir string) func(context.Context, *deleteDirectoryRequest) (*deleteDirectoryResponse, error) {
	return func(_ context.Context, req *deleteDirectoryRequest) (*deleteDirectoryResponse, error) {
		rsp := &deleteDirectoryResponse{BaseDirectory: baseDir, Path: req.Path}
		dirPath, err := resolvePath(baseDir, req.Path)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
		stat, err := os.Stat(dirPath)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: directory not found: %v", err)
			return rsp, fmt.Errorf("directory not found '%s': %w", req.Path, err)
		}
		if !stat.IsDir() {
			rsp.Message = fmt.Sprintf("Error: '%s' is a file, use delete_file instead", req.Path)
			return rsp, fmt.Errorf("'%s' is a file, use delete_file instead", req.Path)
		}
		if req.Recursive {
			if err := os.RemoveAll(dirPath); err != nil {
				rsp.Message = fmt.Sprintf("Error: cannot delete directory: %v", err)
				return rsp, fmt.Errorf("deleting directory '%s': %w", req.Path, err)
			}
		} else {
			if err := os.Remove(dirPath); err != nil {
				rsp.Message = fmt.Sprintf("Error: cannot delete directory (not empty?): %v", err)
				return rsp, fmt.Errorf("deleting directory '%s': %w", req.Path, err)
			}
		}
		rsp.Message = fmt.Sprintf("Successfully deleted directory: %s", req.Path)
		return rsp, nil
	}
}

func deleteDirectoryTool(baseDir string) tool.Tool {
	return function.NewFunctionTool(
		deleteDirectoryHandler(baseDir),
		function.WithName("delete_directory"),
		function.WithDescription("Deletes a directory. "+
			"The 'path' parameter is a relative path from the base directory. "+
			"If 'recursive' is false, only an empty directory can be deleted. "+
			"If 'recursive' is true, the directory and all its contents are deleted."),
	)
}

// --- workspace_tree ---

type workspaceTreeRequest struct {
	Path     string `json:"path" jsonschema:"description=The relative path to list from. Empty or '.' for workspace root."`
	MaxDepth int    `json:"max_depth" jsonschema:"description=Maximum depth to recurse. Default 3."`
}

type workspaceTreeResponse struct {
	BaseDirectory string `json:"base_directory"`
	Tree          string `json:"tree"`
	FileCount     int    `json:"file_count"`
	DirCount      int    `json:"dir_count"`
	Message       string `json:"message"`
}

func workspaceTreeHandler(baseDir string) func(context.Context, *workspaceTreeRequest) (*workspaceTreeResponse, error) {
	return func(_ context.Context, req *workspaceTreeRequest) (*workspaceTreeResponse, error) {
		rsp := &workspaceTreeResponse{BaseDirectory: baseDir}
		targetPath, err := resolvePath(baseDir, req.Path)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
		stat, err := os.Stat(targetPath)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: path not found: %v", err)
			return rsp, fmt.Errorf("path not found '%s': %w", req.Path, err)
		}
		if !stat.IsDir() {
			rsp.Message = fmt.Sprintf("Error: '%s' is a file, not a directory", req.Path)
			return rsp, fmt.Errorf("'%s' is a file, not a directory", req.Path)
		}
		maxDepth := req.MaxDepth
		if maxDepth <= 0 {
			maxDepth = 3
		}
		var sb strings.Builder
		var fileCount, dirCount int
		buildTree(&sb, targetPath, "", 0, maxDepth, &fileCount, &dirCount)
		rsp.Tree = sb.String()
		rsp.FileCount = fileCount
		rsp.DirCount = dirCount
		rsp.Message = fmt.Sprintf("Workspace tree: %d files, %d directories", fileCount, dirCount)
		return rsp, nil
	}
}

func buildTree(sb *strings.Builder, dir, prefix string, depth, maxDepth int, fileCount, dirCount *int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for i, entry := range entries {
		isLast := i == len(entries)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		if entry.IsDir() {
			*dirCount++
			sb.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, connector, entry.Name()))
			if depth+1 < maxDepth {
				nextPrefix := prefix + "│   "
				if isLast {
					nextPrefix = prefix + "    "
				}
				buildTree(sb, filepath.Join(dir, entry.Name()), nextPrefix, depth+1, maxDepth, fileCount, dirCount)
			}
		} else {
			*fileCount++
			info, _ := entry.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			sb.WriteString(fmt.Sprintf("%s%s%s (%d bytes)\n", prefix, connector, entry.Name(), size))
		}
	}
}

func workspaceTreeTool(baseDir string) tool.Tool {
	return function.NewFunctionTool(
		workspaceTreeHandler(baseDir),
		function.WithName("workspace_tree"),
		function.WithDescription("Shows the workspace directory structure as a tree. "+
			"The 'path' parameter is a relative path to start from (empty or '.' for root). "+
			"The 'max_depth' parameter controls recursion depth (default 3). "+
			"Returns a tree-formatted string with file sizes, plus file/directory counts."),
	)
}

// --- clean_workspace ---

type cleanWorkspaceRequest struct {
	Confirm bool `json:"confirm" jsonschema:"description=Must be true to execute. Safety guard against accidental cleanup."`
}

type cleanWorkspaceResponse struct {
	BaseDirectory string   `json:"base_directory"`
	Cleaned       []string `json:"cleaned"`
	Skipped       []string `json:"skipped"`
	Message       string   `json:"message"`
}

var cleanableDirs = []string{"docs", "rtl", "tb", "sim", "synth", "gds"}

func cleanWorkspaceHandler(baseDir string) func(context.Context, *cleanWorkspaceRequest) (*cleanWorkspaceResponse, error) {
	return func(_ context.Context, req *cleanWorkspaceRequest) (*cleanWorkspaceResponse, error) {
		rsp := &cleanWorkspaceResponse{BaseDirectory: baseDir}
		if !req.Confirm {
			rsp.Message = "Error: set 'confirm' to true to execute cleanup"
			return rsp, fmt.Errorf("confirm must be true to execute cleanup")
		}
		for _, dir := range cleanableDirs {
			dirPath := filepath.Join(baseDir, dir)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				rsp.Skipped = append(rsp.Skipped, dir+" (not found)")
				continue
			}
			if err := os.RemoveAll(dirPath); err != nil {
				rsp.Skipped = append(rsp.Skipped, fmt.Sprintf("%s (error: %v)", dir, err))
				continue
			}
			rsp.Cleaned = append(rsp.Cleaned, dir)
		}
		rsp.Message = fmt.Sprintf("Cleaned %d directories, skipped %d. Preserved: data/",
			len(rsp.Cleaned), len(rsp.Skipped))
		return rsp, nil
	}
}

func cleanWorkspaceTool(baseDir string) tool.Tool {
	return function.NewFunctionTool(
		cleanWorkspaceHandler(baseDir),
		function.WithName("clean_workspace"),
		function.WithDescription("Cleans the workspace by removing output directories (docs, rtl, tb, sim, synth, gds) "+
			"while preserving the data/ input directory. "+
			"The 'confirm' parameter must be true to execute (safety guard). "+
			"Use this to reset the workspace before re-running the pipeline."),
	)
}

// --- file_stat ---

type fileStatRequest struct {
	Path string `json:"path" jsonschema:"description=The relative path of the file or directory to inspect."`
}

type fileStatResponse struct {
	BaseDirectory string `json:"base_directory"`
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	IsDir         bool   `json:"is_dir"`
	Size          int64  `json:"size"`
	LineCount     int    `json:"line_count"`
	ModifiedAt    string `json:"modified_at"`
	Message       string `json:"message"`
}

func fileStatHandler(baseDir string) func(context.Context, *fileStatRequest) (*fileStatResponse, error) {
	return func(_ context.Context, req *fileStatRequest) (*fileStatResponse, error) {
		rsp := &fileStatResponse{BaseDirectory: baseDir, Path: req.Path}
		filePath, err := resolvePath(baseDir, req.Path)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			rsp.Exists = false
			rsp.Message = fmt.Sprintf("File not found: %s", req.Path)
			return rsp, nil
		}
		rsp.Exists = true
		rsp.IsDir = stat.IsDir()
		rsp.Size = stat.Size()
		rsp.ModifiedAt = stat.ModTime().Format(time.RFC3339)
		if !stat.IsDir() {
			data, err := os.ReadFile(filePath)
			if err == nil {
				rsp.LineCount = strings.Count(string(data), "\n")
				if len(data) > 0 && data[len(data)-1] != '\n' {
					rsp.LineCount++
				}
			}
		}
		rsp.Message = fmt.Sprintf("Stat: %s (size=%d, lines=%d, modified=%s)",
			req.Path, rsp.Size, rsp.LineCount, rsp.ModifiedAt)
		return rsp, nil
	}
}

func fileStatTool(baseDir string) tool.Tool {
	return function.NewFunctionTool(
		fileStatHandler(baseDir),
		function.WithName("file_stat"),
		function.WithDescription("Returns metadata about a file or directory without reading its contents. "+
			"The 'path' parameter is a relative path from the base directory. "+
			"Returns: exists, is_dir, size (bytes), line_count (for files), modified_at (RFC3339). "+
			"If the file does not exist, returns exists=false with no error."),
	)
}

// --- rename ---

type renameRequest struct {
	OldPath string `json:"old_path" jsonschema:"description=The relative path of the file or directory to rename/move."`
	NewPath string `json:"new_path" jsonschema:"description=The new relative path for the file or directory."`
}

type renameResponse struct {
	BaseDirectory string `json:"base_directory"`
	OldPath       string `json:"old_path"`
	NewPath       string `json:"new_path"`
	Message       string `json:"message"`
}

func renameHandler(baseDir string) func(context.Context, *renameRequest) (*renameResponse, error) {
	return func(_ context.Context, req *renameRequest) (*renameResponse, error) {
		rsp := &renameResponse{BaseDirectory: baseDir, OldPath: req.OldPath, NewPath: req.NewPath}
		oldPath, err := resolvePath(baseDir, req.OldPath)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
		newPath, err := resolvePath(baseDir, req.NewPath)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
		if _, err := os.Stat(oldPath); err != nil {
			rsp.Message = fmt.Sprintf("Error: source not found: %v", err)
			return rsp, fmt.Errorf("source not found '%s': %w", req.OldPath, err)
		}
		parentDir := filepath.Dir(newPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			rsp.Message = fmt.Sprintf("Error: cannot create parent directory: %v", err)
			return rsp, fmt.Errorf("creating parent directory: %w", err)
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			rsp.Message = fmt.Sprintf("Error: cannot rename: %v", err)
			return rsp, fmt.Errorf("renaming '%s' to '%s': %w", req.OldPath, req.NewPath, err)
		}
		rsp.Message = fmt.Sprintf("Successfully renamed '%s' to '%s'", req.OldPath, req.NewPath)
		return rsp, nil
	}
}

func renameTool(baseDir string) tool.Tool {
	return function.NewFunctionTool(
		renameHandler(baseDir),
		function.WithName("rename"),
		function.WithDescription("Renames or moves a file or directory. "+
			"'old_path' is the current relative path, 'new_path' is the target relative path. "+
			"Parent directories for the new path are created automatically. "+
			"Works for both files and directories."),
	)
}

// NewExtraTools returns all custom file operation tools.
func NewExtraTools(baseDir string) []tool.Tool {
	return []tool.Tool{
		deleteFileTool(baseDir),
		deleteDirectoryTool(baseDir),
		renameTool(baseDir),
		workspaceTreeTool(baseDir),
		cleanWorkspaceTool(baseDir),
		fileStatTool(baseDir),
	}
}
