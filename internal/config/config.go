package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Settings struct {
	DBPath               string   `yaml:"db_path"`
	TriggerLocation      string   `yaml:"trigger_location"`
	MCPHTTPAddr          string   `yaml:"mcp_http_addr"`
	MCPHTTPPath          string   `yaml:"mcp_http_path"`
	MCPServerName        string   `yaml:"mcp_server_name"`
	MCPAllowedOrigins    []string `yaml:"mcp_allowed_origins"`
	MCPToken             string   `yaml:"mcp_token"`
	MCPOAuthClientID     string   `yaml:"mcp_oauth_client_id"`
	MCPOAuthClientSecret string   `yaml:"mcp_oauth_client_secret"`
	MCPPublicURL         string   `yaml:"mcp_public_url"`
	RemoteMCPURL         string   `yaml:"remote_mcp_url"`
	RemoteMCPToken       string   `yaml:"remote_mcp_token"`
	// DefaultSession is the session used when CTX_ID is unset.
	DefaultSession string `yaml:"default_session,omitempty"`
	// DefaultSessions maps absolute directory paths to the session used when
	// the working directory is at or below that path; the most specific
	// matching path wins and beats DefaultSession.
	DefaultSessions map[string]string `yaml:"default_sessions,omitempty"`
}

// DefaultSessionFor returns the configured default session for cwd: the
// per-directory mapping with the longest matching path prefix (on path
// boundaries), falling back to the global default_session, or "" if neither
// is configured.
func DefaultSessionFor(settings Settings, cwd string) string {
	best, bestLen := settings.DefaultSession, -1
	for dir, session := range settings.DefaultSessions {
		clean := filepath.Clean(dir)
		if cwd != clean && !strings.HasPrefix(cwd, clean+string(filepath.Separator)) {
			continue
		}
		if len(clean) > bestLen {
			best, bestLen = session, len(clean)
		}
	}
	return best
}

// DefaultSession loads settings and returns the default session for the
// current working directory, or "" if none is configured.
func DefaultSession() string {
	settings, err := LoadSettings()
	if err != nil {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return settings.DefaultSession
	}
	return DefaultSessionFor(settings, cwd)
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	cfgDir := filepath.Join(home, ".config", "ctx")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create config directory %s: %w", cfgDir, err)
	}

	return cfgDir, nil
}

func DBPath() (string, error) {
	settings, cfgDir, err := loadSettings()
	if err != nil {
		return "", err
	}

	if settings.DBPath == "" {
		settings.DBPath = filepath.Join(cfgDir, "ctx.sqlite")
		if err := writeSettings(cfgDir, settings); err != nil {
			return "", err
		}
	}

	if !filepath.IsAbs(settings.DBPath) {
		settings.DBPath = filepath.Join(cfgDir, settings.DBPath)
	}

	if err := os.MkdirAll(filepath.Dir(settings.DBPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s for db path: %w", filepath.Dir(settings.DBPath), err)
	}

	return settings.DBPath, nil
}

func TriggerDir() (string, error) {
	settings, cfgDir, err := loadSettings()
	if err != nil {
		return "", err
	}

	dir := settings.TriggerLocation
	if dir == "" {
		dir = filepath.Join(cfgDir, "triggers")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cfgDir, dir)
	}
	return dir, nil
}

func LoadSettings() (Settings, error) {
	settings, _, err := loadSettings()
	return settings, err
}

func loadSettings() (Settings, string, error) {
	cfgDir, err := Dir()
	if err != nil {
		return Settings{}, "", err
	}
	settingsPath := filepath.Join(cfgDir, "settings.yml")
	var settings Settings

	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := yaml.Unmarshal(data, &settings); err != nil {
			return Settings{}, "", fmt.Errorf("failed to parse settings file %s: %w", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return Settings{}, "", fmt.Errorf("failed to read settings file %s: %w", settingsPath, err)
	}

	return settings, cfgDir, nil
}

func writeSettings(cfgDir string, settings Settings) error {
	settingsPath := filepath.Join(cfgDir, "settings.yml")
	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to encode settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write settings file %s: %w", settingsPath, err)
	}
	return nil
}

func TriggerPath(name string) (string, error) {
	triggerDir, err := TriggerDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(triggerDir, name), nil
}
