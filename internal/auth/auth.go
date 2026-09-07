// Package auth stores Harvia Cognito tokens and loads credentials.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/exitcode"
)

const (
	TokenFile = "token.json"
	EnvFile   = "env"
	DBFile    = "harvia.db"
)

// Tokens is the on-disk cache. Field names match sauna-cloud's Python CLI
// so a token.json can be reused. Values must never be printed.
type Tokens struct {
	IDToken      string  `json:"idToken,omitempty"`
	AccessToken  string  `json:"accessToken,omitempty"`
	RefreshToken string  `json:"refreshToken,omitempty"`
	ExpiresIn    float64 `json:"expiresIn,omitempty"`
	FetchedAt    float64 `json:"fetchedAt,omitempty"`
	Username     string  `json:"username,omitempty"`
}

// Status is a redacted view of the token cache.
type Status struct {
	Present         bool    `json:"present"`
	Username        string  `json:"username,omitempty"`
	HasIDToken      bool    `json:"has_id_token"`
	HasRefreshToken bool    `json:"has_refresh_token"`
	ExpiresInS      float64 `json:"expires_in_s,omitempty"`
	FetchedAt       string  `json:"fetched_at,omitempty"`
	Path            string  `json:"path"`
	Mode            string  `json:"mode,omitempty"`
}

// HomeDir resolves $HARVIA_PP_HOME, --home, or ~/.config/harvia.
func HomeDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if env := os.Getenv("HARVIA_PP_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", exitcode.APIf("home dir: %v", err)
	}
	return filepath.Join(home, ".config", "harvia"), nil
}

// TokenPath is $home/token.json.
func TokenPath(home string) string {
	return filepath.Join(home, TokenFile)
}

// EnsureHome creates the config directory (0700).
func EnsureHome(home string) error {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return exitcode.APIf("create %s: %v", home, err)
	}
	return nil
}

// LoadTokens reads token.json. Missing file is not an error.
func LoadTokens(home string) (*Tokens, error) {
	path := TokenPath(home)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, exitcode.APIf("read token: %v", err)
	}
	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, exitcode.APIf("parse token.json: %v", err)
	}
	return &t, nil
}

// SaveTokens writes token.json with mode 0600.
func SaveTokens(home string, t Tokens) error {
	if err := EnsureHome(home); err != nil {
		return err
	}
	path := TokenPath(home)
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return exitcode.APIf("write token: %v", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return exitcode.APIf("chmod token: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return exitcode.APIf("replace token: %v", err)
	}
	return nil
}

// DeleteTokens removes token.json if present.
func DeleteTokens(home string) error {
	err := os.Remove(TokenPath(home))
	if err != nil && !os.IsNotExist(err) {
		return exitcode.APIf("logout: %v", err)
	}
	return nil
}

// TokenValid reports whether the cached idToken is still usable with a 60s
// margin, matching the Python client.
func TokenValid(t *Tokens, now time.Time, username string) bool {
	if t == nil || t.IDToken == "" {
		return false
	}
	if username != "" && t.Username != "" && t.Username != username {
		return false
	}
	ttl := t.ExpiresIn
	if ttl <= 0 {
		ttl = 3600
	}
	age := now.Sub(time.Unix(int64(t.FetchedAt), 0)).Seconds()
	return age < ttl-60
}

// Inspect returns a redacted status for doctor / auth status.
func Inspect(home string) (Status, error) {
	path := TokenPath(home)
	st := Status{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, exitcode.APIf("stat token: %v", err)
	}
	st.Mode = fmt.Sprintf("%04o", info.Mode().Perm())
	t, err := LoadTokens(home)
	if err != nil {
		return st, err
	}
	if t == nil {
		return st, nil
	}
	st.Present = t.IDToken != "" || t.RefreshToken != ""
	st.Username = t.Username
	st.HasIDToken = t.IDToken != ""
	st.HasRefreshToken = t.RefreshToken != ""
	if t.FetchedAt > 0 {
		st.FetchedAt = time.Unix(int64(t.FetchedAt), 0).UTC().Format(time.RFC3339)
		ttl := t.ExpiresIn
		if ttl <= 0 {
			ttl = 3600
		}
		remain := ttl - time.Since(time.Unix(int64(t.FetchedAt), 0)).Seconds()
		if remain < 0 {
			remain = 0
		}
		st.ExpiresInS = remain
	}
	return st, nil
}

// Creds are login inputs. Password must never be logged.
type Creds struct {
	Username string
	Password string
}

// LoadEnvFile applies KEY=value lines without overriding existing env vars.
// Missing files are ignored unless required is true (explicit --env-file).
func LoadEnvFile(path string, required bool) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil
		}
		return exitcode.APIf("env-file: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}

// ResolveCreds reads username/password from env after optional env-file loads.
// The first path is treated as required when requiredFirst is true (--env-file).
func ResolveCreds(envFiles []string, requiredFirst bool) (Creds, error) {
	for i, f := range envFiles {
		if f == "" {
			continue
		}
		if err := LoadEnvFile(f, requiredFirst && i == 0); err != nil {
			return Creds{}, err
		}
	}
	return Creds{
		Username: strings.TrimSpace(os.Getenv("HARVIA_USERNAME")),
		Password: os.Getenv("HARVIA_PASSWORD"),
	}, nil
}

// DefaultEnvCandidates returns env files to try after --env-file.
func DefaultEnvCandidates(home string) []string {
	var out []string
	if f := os.Getenv("HARVIA_ENV_FILE"); f != "" {
		out = append(out, f)
	}
	if home != "" {
		out = append(out, filepath.Join(home, EnvFile))
	}
	return out
}

// RedactSecrets replaces known secret values in s. Used as a last-resort
// filter on error strings so an upstream body cannot leak tokens.
func RedactSecrets(s string, extra ...string) string {
	repl := []string{}
	for _, v := range extra {
		if v != "" && len(v) >= 4 {
			repl = append(repl, v, "[redacted]")
		}
	}
	if len(repl) == 0 {
		return s
	}
	return strings.NewReplacer(repl...).Replace(s)
}
