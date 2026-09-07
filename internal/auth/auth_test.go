package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadInspectRedactsAndMode600(t *testing.T) {
	dir := t.TempDir()
	tok := Tokens{
		IDToken:      "secret-id-token",
		AccessToken:  "secret-access",
		RefreshToken: "secret-refresh",
		ExpiresIn:    3600,
		FetchedAt:    float64(time.Now().Unix()),
		Username:     "fixture@example.com",
	}
	if err := SaveTokens(dir, tok); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(TokenPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o want 0600", info.Mode().Perm())
	}
	got, err := LoadTokens(dir)
	if err != nil || got == nil {
		t.Fatalf("load: %v %#v", err, got)
	}
	if got.IDToken != tok.IDToken {
		t.Fatal("id token mismatch")
	}
	st, err := Inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Present || !st.HasIDToken || !st.HasRefreshToken {
		t.Fatalf("status flags: %+v", st)
	}
	if strings.Contains(st.Username, "secret") {
		t.Fatal("username should not look like a token")
	}
	enc, _ := os.ReadFile(TokenPath(dir))
	if !strings.Contains(string(enc), "secret-id-token") {
		t.Fatal("file should store the token")
	}
}

func TestTokenValidMargin(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	t.Run("fresh", func(t *testing.T) {
		tok := &Tokens{IDToken: "x", ExpiresIn: 3600, FetchedAt: float64(now.Unix()), Username: "a"}
		if !TokenValid(tok, now.Add(10*time.Second), "a") {
			t.Fatal("expected valid")
		}
	})
	t.Run("within margin", func(t *testing.T) {
		tok := &Tokens{IDToken: "x", ExpiresIn: 3600, FetchedAt: float64(now.Unix())}
		if TokenValid(tok, now.Add(3550*time.Second), "") {
			t.Fatal("should be invalid inside 60s margin")
		}
	})
	t.Run("wrong user", func(t *testing.T) {
		tok := &Tokens{IDToken: "x", ExpiresIn: 3600, FetchedAt: float64(now.Unix()), Username: "a"}
		if TokenValid(tok, now, "b") {
			t.Fatal("wrong user")
		}
	})
}

func TestLoadEnvFileDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte("HARVIA_USERNAME=fromfile\nHARVIA_PASSWORD=pw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARVIA_USERNAME", "fromenv")
	t.Setenv("HARVIA_PASSWORD", "")
	_ = os.Unsetenv("HARVIA_PASSWORD")
	if err := LoadEnvFile(path, true); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HARVIA_USERNAME") != "fromenv" {
		t.Fatal("env should win")
	}
	if os.Getenv("HARVIA_PASSWORD") != "pw" {
		t.Fatalf("password from file: %q", os.Getenv("HARVIA_PASSWORD"))
	}
}

func TestRedactSecrets(t *testing.T) {
	s := RedactSecrets("got secret-id-token in body", "secret-id-token")
	if strings.Contains(s, "secret-id-token") {
		t.Fatal(s)
	}
}
