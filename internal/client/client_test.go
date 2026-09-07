package client

import (
	"os"
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
