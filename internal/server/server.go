package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"k-synaptic/internal/config"
	"k-synaptic/internal/filesystem"
	"k-synaptic/internal/handlers"
	"net"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	config       *config.Config
	fs           *filesystem.Manager
	router       *http.ServeMux
	listener     net.Listener
	sessions     map[string]time.Time
	sessionMutex sync.RWMutex
}

func New(cfg *config.Config) *Server {
	fs := filesystem.NewManager(cfg.Include, cfg.Exclude)
	router := http.NewServeMux()

	return &Server{
		config:   cfg,
		fs:       fs,
		router:   router,
		sessions: make(map[string]time.Time),
	}
}

func (s *Server) generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) setSession(w http.ResponseWriter, token string) {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	s.sessions[token] = time.Now().Add(24 * time.Hour)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

func (s *Server) isSessionValid(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}

	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()

	expiry, exists := s.sessions[cookie.Value]
	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		return false
	}

	return true
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		s.sessionMutex.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMutex.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (s *Server) setupRoutes(h *handlers.Handler) {
	s.router.HandleFunc("/login", s.loginHandler)
	s.router.HandleFunc("/logout", s.logoutHandler)
	s.router.HandleFunc("/", s.indexHandler)
	s.router.HandleFunc("/api/list", s.authMiddleware(h.ListFiles))
	s.router.HandleFunc("/api/download", s.authMiddleware(h.DownloadFile))
	s.router.HandleFunc("/api/upload", s.authMiddleware(h.UploadFile))
	s.router.HandleFunc("/api/delete", s.authMiddleware(h.DeleteFile))
	s.router.HandleFunc("/api/rename", s.authMiddleware(h.RenameFile))
	s.router.HandleFunc("/api/mkdir", s.authMiddleware(h.CreateDirectory))
	s.router.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, loginHTML)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "bhavyam" && password == "tenebris0901" {
			token := s.generateSessionToken()
			s.setSession(w, token)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, loginHTMLWithError)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if !s.isSessionValid(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, indexHTML)
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isSessionValid(r) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
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

const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - k-synaptic</title>
    <style>
        :root {
            --bg-primary: #0a0e27;
            --bg-secondary: #1a1f3a;
            --text-primary: #e8eaf0;
            --accent: #6366f1;
            --border: #3f4556;
        }
        
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        html, body {
            width: 100%;
            height: 100%;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, var(--bg-primary) 0%, var(--bg-secondary) 100%);
            display: flex;
            align-items: center;
            justify-content: center;
        }
        
        .login-container {
            background-color: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 40px;
            max-width: 400px;
            width: 90%;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
        }
        
        .login-header {
            text-align: center;
            margin-bottom: 30px;
        }
        
        .login-header h1 {
            font-size: 2em;
            color: var(--accent);
            margin-bottom: 10px;
            letter-spacing: 1px;
        }
        
        .login-header p {
            color: #a0a8b8;
            font-size: 0.9em;
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        label {
            display: block;
            color: var(--text-primary);
            font-weight: 500;
            margin-bottom: 8px;
            font-size: 0.95em;
        }
        
        input {
            width: 100%;
            padding: 12px;
            background-color: var(--bg-primary);
            border: 1px solid var(--border);
            border-radius: 6px;
            color: var(--text-primary);
            font-size: 1em;
            transition: all 0.2s ease;
        }
        
        input:focus {
            outline: none;
            border-color: var(--accent);
            background-color: #0f1432;
            box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
        }
        
        .login-btn {
            width: 100%;
            padding: 12px;
            background-color: var(--accent);
            color: white;
            border: none;
            border-radius: 6px;
            font-size: 1em;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s ease;
            margin-top: 10px;
        }
        
        .login-btn:hover {
            background-color: #818cf8;
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
        }
        
        .login-btn:active {
            transform: translateY(0);
        }
        
        .footer {
            text-align: center;
            margin-top: 20px;
            font-size: 0.8em;
            color: #a0a8b8;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <div class="login-header">
            <h1>k-synaptic</h1>
            <p>Secure File Manager</p>
        </div>
        
        <form method="POST">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required autofocus>
            </div>
            
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            
            <button type="submit" class="login-btn">Login</button>
        </form>
        
        <div class="footer">
            <p>Secure access only</p>
        </div>
    </div>
</body>
</html>`

const loginHTMLWithError = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - k-synaptic</title>
    <style>
        :root {
            --bg-primary: #0a0e27;
            --bg-secondary: #1a1f3a;
            --text-primary: #e8eaf0;
            --accent: #6366f1;
            --border: #3f4556;
            --error: #ef4444;
        }
        
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        html, body {
            width: 100%;
            height: 100%;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, var(--bg-primary) 0%, var(--bg-secondary) 100%);
            display: flex;
            align-items: center;
            justify-content: center;
        }
        
        .login-container {
            background-color: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 12px;
            padding: 40px;
            max-width: 400px;
            width: 90%;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
        }
        
        .login-header {
            text-align: center;
            margin-bottom: 30px;
        }
        
        .login-header h1 {
            font-size: 2em;
            color: var(--accent);
            margin-bottom: 10px;
            letter-spacing: 1px;
        }
        
        .login-header p {
            color: #a0a8b8;
            font-size: 0.9em;
        }
        
        .error-message {
            background-color: rgba(239, 68, 68, 0.1);
            border: 1px solid var(--error);
            border-radius: 6px;
            padding: 12px;
            margin-bottom: 20px;
            color: #fca5a5;
            font-size: 0.9em;
            text-align: center;
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        label {
            display: block;
            color: var(--text-primary);
            font-weight: 500;
            margin-bottom: 8px;
            font-size: 0.95em;
        }
        
        input {
            width: 100%;
            padding: 12px;
            background-color: var(--bg-primary);
            border: 1px solid var(--border);
            border-radius: 6px;
            color: var(--text-primary);
            font-size: 1em;
            transition: all 0.2s ease;
        }
        
        input:focus {
            outline: none;
            border-color: var(--accent);
            background-color: #0f1432;
            box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
        }
        
        .login-btn {
            width: 100%;
            padding: 12px;
            background-color: var(--accent);
            color: white;
            border: none;
            border-radius: 6px;
            font-size: 1em;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s ease;
            margin-top: 10px;
        }
        
        .login-btn:hover {
            background-color: #818cf8;
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
        }
        
        .login-btn:active {
            transform: translateY(0);
        }
        
        .footer {
            text-align: center;
            margin-top: 20px;
            font-size: 0.8em;
            color: #a0a8b8;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <div class="login-header">
            <h1>k-synaptic</h1>
            <p>Secure File Manager</p>
        </div>
        
        <div class="error-message">
            Invalid username or password
        </div>
        
        <form method="POST">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required autofocus>
            </div>
            
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            
            <button type="submit" class="login-btn">Login</button>
        </form>
        
        <div class="footer">
            <p>Secure access only</p>
        </div>
    </div>
</body>
</html>`

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
                    <a href="/logout" class="btn btn-logout" style="text-decoration:none;display:flex;align-items:center;">Logout</a>
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
