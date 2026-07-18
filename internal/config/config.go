// Package config manages wt's JSON configuration file.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	wterrors "wt/internal/errors"
)

// AppName is the binary/config directory name.
const AppName = "wt"

// DefaultConfig is deep-merged under the user's config file.
var DefaultConfig = map[string]any{
	"ai_tool": map[string]any{
		"command": "claude",
		"args":    []any{},
	},
	"launch": map[string]any{
		"method":                nil,
		"session_prefix":        AppName,
		"wezterm_ready_timeout": 5.0,
	},
	"git": map[string]any{
		"default_base_branch": "main",
	},
	"session": map[string]any{
		"auto_resume": false,
	},
	"shell_completion": map[string]any{
		"prompted":  false,
		"installed": false,
	},
}

// Dir returns the wt config directory (~/.config/wt, XDG-aware).
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "."+AppName)
	}
	return filepath.Join(home, ".config", AppName)
}

// Path returns the config file path.
func Path() string {
	return filepath.Join(Dir(), "config.json")
}

// Load reads the config file deep-merged over defaults.
// A missing file returns the defaults.
func Load() (map[string]any, error) {
	cfg := deepCopy(DefaultConfig)
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, wterrors.Wrap(wterrors.ErrConfig, err, "failed to read config file")
	}
	var user map[string]any
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, wterrors.Wrap(wterrors.ErrConfig, err, "invalid JSON in config file %s", Path())
	}
	deepMerge(cfg, user)
	return cfg, nil
}

// Save writes the config atomically (temp file + rename).
func Save(cfg map[string]any) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(Dir(), "config-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, Path())
}

// Get returns the value at a dot path ("launch.session_prefix"), or nil.
func Get(cfg map[string]any, dotPath string) any {
	cur := cfg
	for _, part := range strings.Split(dotPath, ".") {
		m, ok := cur[part].(map[string]any)
		if !ok {
			// last segment may be a leaf
			return cur[part]
		}
		cur = m
	}
	return cur
}

// Set assigns a value at a dot path, creating intermediate maps.
// "true"/"false" (case-insensitive) coerce to bool.
func Set(cfg map[string]any, dotPath, raw string) {
	parts := strings.Split(dotPath, ".")
	cur := cfg
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = coerce(raw)
}

func coerce(raw string) any {
	switch strings.ToLower(raw) {
	case "true":
		return true
	case "false":
		return false
	}
	return raw
}

// GetString is a typed accessor for string values.
func GetString(cfg map[string]any, dotPath string) string {
	if s, ok := Get(cfg, dotPath).(string); ok {
		return s
	}
	return ""
}

// GetBool is a typed accessor for bool values.
func GetBool(cfg map[string]any, dotPath string) bool {
	b, _ := Get(cfg, dotPath).(bool)
	return b
}

// AutoResume reports whether launching should automatically continue an
// existing AI session. Priority: WT_AUTO_RESUME env (1/true/yes enables,
// 0/false/no disables) > config session.auto_resume (default false).
func AutoResume(cfg map[string]any) bool {
	switch strings.ToLower(envLookup("WT_AUTO_RESUME")) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return GetBool(cfg, "session.auto_resume")
}

// GetFloat is a typed accessor for numeric values.
func GetFloat(cfg map[string]any, dotPath string) float64 {
	f, _ := Get(cfg, dotPath).(float64)
	return f
}

func deepCopy(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		if m, ok := v.(map[string]any); ok {
			out[k] = deepCopy(m)
		} else {
			out[k] = v
		}
	}
	return out
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]any); ok {
			if dstMap, ok := dst[k].(map[string]any); ok {
				deepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}
