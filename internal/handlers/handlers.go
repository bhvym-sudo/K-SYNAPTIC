package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"k-synaptic/internal/config"
	"k-synaptic/internal/filesystem"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Handler struct {
	fs     *filesystem.Manager
	config *config.Config
}

func New(fs *filesystem.Manager, cfg *config.Config) *Handler {
	return &Handler{
		fs:     fs,
		config: cfg,
	}
}

type ListResponse struct {
	Files []*filesystem.FileInfo `json:"files"`
	Path  string                 `json:"path"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Query().Get("path")
	if path == "" {
		if len(h.config.Include) > 0 {
			path = h.config.Include[0]
		} else {
			path = "."
		}
	}

	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	if !h.fs.IsPathAllowed(path) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	files, err := h.fs.ListDirectory(path)
	if err != nil {
		http.Error(w, `{"error":"failed to list directory"}`, http.StatusInternalServerError)
		return
	}

	resp := ListResponse{
		Files: files,
		Path:  path,
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, `{"error":"missing path"}`, http.StatusBadRequest)
		return
	}

	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	if !h.fs.IsPathAllowed(path) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	file, err := h.fs.ReadFile(path)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}
	defer file.Close()

	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	io.Copy(w, file)
}

func (h *Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	path := r.FormValue("path")
	if path == "" {
		if len(h.config.Include) > 0 {
			path = h.config.Include[0]
		} else {
			path = "."
		}
	}

	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	if !h.fs.IsPathAllowed(path) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"upload failed"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	filePath := filepath.Join(path, handler.Filename)
	if !h.fs.IsPathAllowed(filePath) {
		http.Error(w, `{"error":"invalid target path"}`, http.StatusForbidden)
		return
	}

	if err := h.fs.WriteFile(filePath, file); err != nil {
		http.Error(w, `{"error":"write failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	path := r.FormValue("path")
	if path == "" {
		http.Error(w, `{"error":"missing path"}`, http.StatusBadRequest)
		return
	}

	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	if !h.fs.IsPathAllowed(path) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	if err := h.fs.DeletePath(path); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}

func (h *Handler) RenameFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	oldPath := r.FormValue("old_path")
	newName := r.FormValue("new_name")

	if oldPath == "" || newName == "" {
		http.Error(w, `{"error":"missing parameters"}`, http.StatusBadRequest)
		return
	}

	oldPath = filepath.Clean(oldPath)
	if strings.Contains(oldPath, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	if !h.fs.IsPathAllowed(oldPath) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	dir := filepath.Dir(oldPath)
	newPath := filepath.Join(dir, newName)

	if !h.fs.IsPathAllowed(newPath) {
		http.Error(w, `{"error":"invalid target path"}`, http.StatusForbidden)
		return
	}

	if err := h.fs.RenamePath(oldPath, newPath); err != nil {
		http.Error(w, `{"error":"rename failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}

func (h *Handler) CreateDirectory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	path := r.FormValue("path")
	if path == "" {
		http.Error(w, `{"error":"missing path"}`, http.StatusBadRequest)
		return
	}

	path = filepath.Clean(path)
	if strings.Contains(path, "..") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	if !h.fs.IsPathAllowed(path) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	if err := h.fs.CreateDirectory(path); err != nil {
		http.Error(w, `{"error":"mkdir failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}
