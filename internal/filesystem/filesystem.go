package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileInfo struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Type         string    `json:"type"`
	Size         int64     `json:"size"`
	ModifiedTime time.Time `json:"modified_time"`
	IsDir        bool      `json:"is_dir"`
}

type Manager struct {
	allowedPaths []string
	excludePaths []string
}

func NewManager(allowed []string, exclude []string) *Manager {
	normalized := make([]string, len(allowed))
	for i, p := range allowed {
		normalized[i] = filepath.Clean(p)
	}

	excludeNorm := make([]string, len(exclude))
	for i, p := range exclude {
		excludeNorm[i] = filepath.Clean(p)
	}

	return &Manager{
		allowedPaths: normalized,
		excludePaths: excludeNorm,
	}
}

func (m *Manager) IsPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absPath = filepath.Clean(absPath)

	for _, excludePath := range m.excludePaths {
		if m.isPathUnderDir(absPath, excludePath) {
			return false
		}
	}

	if len(m.allowedPaths) == 0 {
		return false
	}

	for _, allowPath := range m.allowedPaths {
		if m.isPathUnderDir(absPath, allowPath) || absPath == allowPath {
			return true
		}
	}

	return false
}

func (m *Manager) isPathUnderDir(path, dir string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)

	if path == dir {
		return true
	}

	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}

	return !strings.HasPrefix(rel, "..")
}

func (m *Manager) SanitizePath(basePath string, requestedPath string) (string, error) {
	if requestedPath == "" {
		requestedPath = "."
	}

	requestedPath = filepath.Clean(requestedPath)

	for strings.Contains(requestedPath, "..") {
		requestedPath = strings.ReplaceAll(requestedPath, "..", "")
	}

	fullPath := filepath.Join(basePath, requestedPath)
	fullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	fullPath = filepath.Clean(fullPath)

	realPath, err := filepath.EvalSymlinks(fullPath)
	if err == nil {
		fullPath = realPath
	}

	if !m.IsPathAllowed(fullPath) {
		return "", os.ErrPermission
	}

	return fullPath, nil
}

func (m *Manager) ListDirectory(path string) ([]*FileInfo, error) {
	path = filepath.Clean(path)

	if !m.IsPathAllowed(path) {
		return nil, os.ErrPermission
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var files []*FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := filepath.Join(path, entry.Name())
		if !m.IsPathAllowed(entryPath) {
			continue
		}

		fileType := "file"
		if entry.IsDir() {
			fileType = "folder"
		}

		files = append(files, &FileInfo{
			Name:         entry.Name(),
			Path:         entryPath,
			Type:         fileType,
			Size:         info.Size(),
			ModifiedTime: info.ModTime(),
			IsDir:        entry.IsDir(),
		})
	}

	return files, nil
}

func (m *Manager) GetFileInfo(path string) (*FileInfo, error) {
	path = filepath.Clean(path)

	if !m.IsPathAllowed(path) {
		return nil, os.ErrPermission
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	fileType := "file"
	if info.IsDir() {
		fileType = "folder"
	}

	return &FileInfo{
		Name:         filepath.Base(path),
		Path:         path,
		Type:         fileType,
		Size:         info.Size(),
		ModifiedTime: info.ModTime(),
		IsDir:        info.IsDir(),
	}, nil
}

func (m *Manager) DeletePath(path string) error {
	path = filepath.Clean(path)

	if !m.IsPathAllowed(path) {
		return os.ErrPermission
	}

	return os.RemoveAll(path)
}

func (m *Manager) RenamePath(oldPath string, newPath string) error {
	oldPath = filepath.Clean(oldPath)
	newPath = filepath.Clean(newPath)

	if !m.IsPathAllowed(oldPath) || !m.IsPathAllowed(newPath) {
		return os.ErrPermission
	}

	return os.Rename(oldPath, newPath)
}

func (m *Manager) CreateDirectory(path string) error {
	path = filepath.Clean(path)

	if !m.IsPathAllowed(path) {
		return os.ErrPermission
	}

	return os.MkdirAll(path, 0755)
}

func (m *Manager) ReadFile(path string) (io.ReadCloser, error) {
	path = filepath.Clean(path)

	if !m.IsPathAllowed(path) {
		return nil, os.ErrPermission
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, os.ErrInvalid
	}

	return os.Open(path)
}

func (m *Manager) WriteFile(path string, content io.Reader) error {
	path = filepath.Clean(path)

	if !m.IsPathAllowed(path) {
		return os.ErrPermission
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, content)
	return err
}
