package client

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/amansk/harvia-pp-cli/internal/fixture"
)

func TestDeviceUUIDPrefersName(t *testing.T) {
	d := Device{
		DeviceID: "not-the-uuid",
		ID:       "also-wrong",
		Name:     "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
	}
	if got := DeviceUUID(d); got != d.Name {
		t.Fatalf("got %s want name", got)
	}
	d.Name = "friendly-label"
	if got := DeviceUUID(d); got != "not-the-uuid" {
		t.Fatalf("fallback deviceId: %s", got)
	}
}

func TestLoginRefreshAndDeviceID(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	home := t.TempDir()
	c := &Client{
		EndpointsURL: srv.EndpointsURL,
		Home:         home,
		Username:     "fixture@example.com",
		Password:     "fixture-password-not-real",
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	tok, err := c.Login()
	if err != nil {
		t.Fatal(err)
	}
	if tok.IDToken != "fixture-id-token" || tok.RefreshToken != "fixture-refresh-token" {
		t.Fatalf("tokens: %+v", tok)
	}
	info, err := os.Stat(auth.TokenPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}

	id, err := c.DeviceID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" {
		t.Fatalf("device id %s", id)
	}

	// Force refresh: expire both in-memory and on-disk tokens.
	expired := *c.tokens
	expired.FetchedAt = float64(time.Unix(1_600_000_000, 0).Unix())
	if err := auth.SaveTokens(home, expired); err != nil {
		t.Fatal(err)
	}
	c.tokens = &expired
	got, err := c.IDToken(false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fixture-id-token-2" {
		t.Fatalf("refreshed id %s", got)
	}
	cached, err := auth.LoadTokens(home)
	if err != nil {
		t.Fatal(err)
	}
	if cached.RefreshToken != "fixture-refresh-token" {
		t.Fatalf("refresh must keep old refresh token, got %q", cached.RefreshToken)
	}
	if cached.IDToken != "fixture-id-token-2" {
		t.Fatal("id token should update")
	}
}

func TestLoginKeepsSSOHint(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	srv.FailAuth = true
	c := &Client{
		EndpointsURL: srv.EndpointsURL,
		Home:         t.TempDir(),
		Username:     "fixture@example.com",
		Password:     "wrong",
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	_, err := c.Login()
	if err == nil {
		t.Fatal("expected login error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Apple/Google SSO") {
		t.Fatalf("SSO hint dropped: %s", msg)
	}
	if !strings.Contains(msg, "wrong password") {
		t.Fatalf("password hint dropped: %s", msg)
	}
}

func TestAuthed401RefreshThenRelogin(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	home := t.TempDir()
	c := &Client{
		EndpointsURL: srv.EndpointsURL,
		Home:         home,
		Username:     "fixture@example.com",
		Password:     "",
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	c.Password = "fixture-password-not-real"
	if _, err := c.Login(); err != nil {
		t.Fatal(err)
	}
	c.Password = "" // token-only session
	srv.UnauthorizedDevices = 1
	devs, err := c.Devices()
	if err != nil {
		t.Fatalf("token-only 401 should refresh: %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("devices %d", len(devs))
	}
	var sawRefresh bool
	for _, r := range srv.Requests() {
		if r.Path == "/auth/refresh" {
			sawRefresh = true
		}
	}
	if !sawRefresh {
		t.Fatal("expected refresh after 401")
	}
	if c.tokens == nil || c.tokens.IDToken != "fixture-id-token-2" {
		t.Fatalf("expected refreshed id token, got %+v", c.tokens)
	}
}

func TestAuthed401RefreshFailThenRelogin(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	c := &Client{
		EndpointsURL: srv.EndpointsURL,
		Home:         t.TempDir(),
		Username:     "fixture@example.com",
		Password:     "fixture-password-not-real",
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	if _, err := c.Login(); err != nil {
		t.Fatal(err)
	}
	srv.FailRefresh = true
	srv.UnauthorizedDevices = 1
	if _, err := c.Devices(); err != nil {
		t.Fatalf("re-login should recover after failed refresh: %v", err)
	}
	var refreshN, loginN int
	for _, r := range srv.Requests() {
		if r.Path == "/auth/refresh" {
			refreshN++
		}
		if r.Path == "/auth/token" {
			loginN++
		}
	}
	if refreshN < 1 || loginN < 2 {
		t.Fatalf("expected refresh then re-login, refresh=%d login=%d", refreshN, loginN)
	}
}

func TestAuthed401ReturnsRecoveryError(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	c := &Client{
		EndpointsURL: srv.EndpointsURL,
		Home:         t.TempDir(),
		Username:     "fixture@example.com",
		Password:     "fixture-password-not-real",
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	if _, err := c.Login(); err != nil {
		t.Fatal(err)
	}
	c.Password = ""
	srv.FailRefresh = true
	srv.UnauthorizedDevices = 1
	_, err := c.Devices()
	if err == nil {
		t.Fatal("expected recovery error")
	}
	msg := err.Error()
	if strings.Contains(msg, "/devices") && !strings.Contains(msg, "refresh") {
		t.Fatalf("should return refresh/re-login error, not original 401: %s", msg)
	}
	if !strings.Contains(msg, "refresh") && !strings.Contains(msg, "401") {
		t.Fatalf("expected refresh failure, got %s", msg)
	}
}

func TestParseStateTelemetryFlexibleTypes(t *testing.T) {
	st := parseState([]byte(`{"state":{"displayName":"X","heater":{"on":1},"activeProfile":3,"targetTemp":82}}`))
	if !st.HeaterOn || st.ActiveProfile != "3" || st.TargetTemp == nil || *st.TargetTemp != 82 {
		t.Fatalf("%+v", st)
	}
	tele := parseTelemetry([]byte(`{"data":{"temp":42,"lightOn":1,"fanOn":false,"doorSafetyState":1}}`))
	if tele.Temp == nil || *tele.Temp != 42 || !tele.LightOn || tele.FanOn {
		t.Fatalf("%+v", tele)
	}
	if tele.DoorSafetyState == nil || *tele.DoorSafetyState != 1 {
		t.Fatal("door")
	}
}
