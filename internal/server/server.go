package server

import (
	"fmt"
	"k-synaptic/internal/config"
	"k-synaptic/internal/filesystem"
	"k-synaptic/internal/handlers"
	"net"
	"net/http"
)

type Server struct {
	config   *config.Config
	fs       *filesystem.Manager
	router   *http.ServeMux
	listener net.Listener
}

func New(cfg *config.Config) *Server {
	fs := filesystem.NewManager(cfg.Include, cfg.Exclude)
	router := http.NewServeMux()

	return &Server{
		config: cfg,
		fs:     fs,
		router: router,
	}
}

func (s *Server) setupRoutes(h *handlers.Handler) {
	s.router.HandleFunc("/", s.indexHandler)
	s.router.HandleFunc("/api/list", s.authMiddleware(h.ListFiles))
	s.router.HandleFunc("/api/download", s.authMiddleware(h.DownloadFile))
	s.router.HandleFunc("/api/upload", s.authMiddleware(h.UploadFile))
	s.router.HandleFunc("/api/delete", s.authMiddleware(h.DeleteFile))
	s.router.HandleFunc("/api/rename", s.authMiddleware(h.RenameFile))
	s.router.HandleFunc("/api/mkdir", s.authMiddleware(h.CreateDirectory))
	s.router.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if s.config.Username != "" && s.config.Password != "" {
		username, password, ok := r.BasicAuth()
		if !ok || username != s.config.Username || password != s.config.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="k-synaptic"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, indexHTML)
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.Username != "" && s.config.Password != "" {
			username, password, ok := r.BasicAuth()
			if !ok || username != s.config.Username || password != s.config.Password {
				w.Header().Set("WWW-Authenticate", `Basic realm="k-synaptic"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) localNetworkOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}

		ip := net.ParseIP(host)
		if ip == nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !isLocalNetwork(ip) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isLocalNetwork(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if ip.IsPrivate() {
		return true
	}

	if ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}

	return false
}

func (s *Server) Start() error {
	h := handlers.New(s.fs, s.config)
	s.setupRoutes(h)

	addr := fmt.Sprintf("0.0.0.0:%d", s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = listener

	wrappedHandler := s.localNetworkOnly(s.router)

	fmt.Printf("Server starting on http://localhost:%d\n", s.config.Port)

	ips := getLocalIPs()
	if len(ips) > 0 {
		fmt.Println("WiFi accessible at:")
		for _, ip := range ips {
			fmt.Printf("  http://%s:%d\n", ip, s.config.Port)
		}
	}

	if s.config.CertFile != "" && s.config.KeyFile != "" {
		return http.ServeTLS(listener, wrappedHandler, s.config.CertFile, s.config.KeyFile)
	}

	return http.Serve(listener, wrappedHandler)
}

func getLocalIPs() []string {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP.To4()
		if ip == nil {
			continue
		}

		if ip.IsLoopback() {
			continue
		}

		if ip.IsPrivate() {
			ips = append(ips, ip.String())
		}
	}

	return ips
}

func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>k-synaptic</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <aside class="sidebar">
            <div class="sidebar-header">
                <h1>k-synaptic</h1>
            </div>
            <div class="sidebar-roots" id="sidebarRoots">
            </div>
        </aside>
        
        <main class="content">
            <div class="header">
                <div class="breadcrumb" id="breadcrumb">
                    <span class="breadcrumb-item active">Home</span>
                </div>
                <div class="actions">
                    <button id="uploadBtn" class="btn btn-primary">Upload</button>
                    <button id="newFolderBtn" class="btn btn-secondary">New Folder</button>
                </div>
            </div>
            
            <div class="file-list-container">
                <div class="file-list" id="fileList">
                    <div class="loading">Loading...</div>
                </div>
            </div>
            
            <div class="upload-zone" id="uploadZone">
                <div class="upload-content">
                    <p>Drop files here to upload</p>
                </div>
                <input type="file" id="hiddenUploadInput" multiple style="display:none;">
            </div>
        </main>
    </div>
    
    <script src="/static/app.js"></script>
</body>
</html>`
