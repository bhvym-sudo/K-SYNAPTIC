package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	Port      int
	Include   []string
	Exclude   []string
	Username  string
	Password  string
	CertFile  string
	KeyFile   string
	AllowRoot bool
}

func Parse() (*Config, error) {
	cfg := &Config{
		Port:    8080,
		Include: []string{},
		Exclude: []string{},
	}

	include := flag.String("include", "", "Directories to include (comma-separated)")
	exclude := flag.String("exclude", "", "Directories to exclude (comma-separated)")
	port := flag.Int("port", 8080, "Port to listen on")
	username := flag.String("username", "", "Basic auth username")
	password := flag.String("password", "", "Basic auth password")
	certFile := flag.String("cert", "", "TLS certificate file")
	keyFile := flag.String("key", "", "TLS key file")

	flag.Parse()

	cfg.Port = *port
	cfg.Username = *username
	cfg.Password = *password
	cfg.CertFile = *certFile
	cfg.KeyFile = *keyFile

	if *include == "" && *exclude == "" {
		cfg.AllowRoot = true
		cfg.Include = getDriveRoots()
	} else if *include != "" {
		parts := strings.Split(*include, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				abs, err := filepath.Abs(p)
				if err == nil {
					cfg.Include = append(cfg.Include, abs)
				}
			}
		}
	}

	if *exclude != "" {
		parts := strings.Split(*exclude, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				abs, err := filepath.Abs(p)
				if err == nil {
					cfg.Exclude = append(cfg.Exclude, abs)
				}
			}
		}
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", cfg.Port)
	}

	if len(cfg.Include) == 0 {
		cfg.Include = getDriveRoots()
		cfg.AllowRoot = true
	}

	return cfg, nil
}

func getDriveRoots() []string {
	var drives []string

	if runtime.GOOS == "windows" {
		for i := 'A'; i <= 'Z'; i++ {
			drive := string(i) + ":"
			if _, err := os.Stat(drive + "\\"); err == nil {
				drives = append(drives, drive+"\\")
			}
		}
	} else {
		drives = []string{"/home"}
	}

	return drives
}

func (c *Config) IsPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absPath = filepath.Clean(absPath)

	for _, excludePath := range c.Exclude {
		excludePath = filepath.Clean(excludePath)
		if strings.HasPrefix(absPath, excludePath) {
			return false
		}
	}

	if c.AllowRoot {
		return true
	}

	if len(c.Include) == 0 {
		return false
	}

	for _, includePath := range c.Include {
		includePath = filepath.Clean(includePath)
		if strings.HasPrefix(absPath, includePath) {
			return true
		}
	}

	return false
}
