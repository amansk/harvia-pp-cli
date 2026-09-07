package actions

import (
	"testing"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/client"
	"github.com/amansk/harvia-pp-cli/internal/fixture"
)

func newClient(t *testing.T, srv *fixture.Server) *client.Client {
	t.Helper()
	home := t.TempDir()
	c := &client.Client{
		EndpointsURL: srv.EndpointsURL,
		Home:         home,
		Username:     "fixture@example.com",
		Password:     "fixture-password-not-real",
		Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	if _, err := c.Login(); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestWarmupCustomSlotOrder(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	c := newClient(t, srv)
	temp := 82
	if _, err := Warmup(c, &temp, nil); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, r := range srv.Requests() {
		if r.Path == "/devices/target" || r.Path == "/devices/profile" || r.Path == "/devices/command" {
			paths = append(paths, r.Method+" "+r.Path)
		}
	}
	want := []string{"PATCH /devices/target", "PATCH /devices/profile", "POST /devices/command"}
	if len(paths) < 3 {
		t.Fatalf("writes %v", paths)
	}
	// Last three writes after login/reads.
	got := paths[len(paths)-3:]
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("order %v want %v", got, want)
		}
	}
	var target, profile map[string]any
	for _, r := range srv.Requests() {
		if r.Path == "/devices/target" {
			target = r.Body
		}
		if r.Path == "/devices/profile" {
			profile = r.Body
		}
	}
	if target["profile"] != "3" || target["temperature"] != float64(82) {
		t.Fatalf("target body %+v", target)
	}
	if profile["profile"] != "3" {
		t.Fatalf("profile body %+v", profile)
	}
}

func TestSetTempMidSessionVsOff(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	c := newClient(t, srv)

	if _, mid, err := SetTemp(c, 85); err != nil || mid {
		t.Fatalf("off heater should bare-patch mid=%v err=%v", mid, err)
	}
	var sawProfile bool
	for _, r := range srv.Requests() {
		if r.Path == "/devices/profile" {
			sawProfile = true
		}
	}
	if sawProfile {
		t.Fatal("heater off should not activate Custom slot")
	}

	srv.StateOn = true
	if _, mid, err := SetTemp(c, 88); err != nil || !mid {
		t.Fatalf("on heater should use custom slot mid=%v err=%v", mid, err)
	}
	sawProfile = false
	for _, r := range srv.Requests() {
		if r.Path == "/devices/profile" {
			sawProfile = true
		}
	}
	if !sawProfile {
		t.Fatal("mid-session temp must PATCH profile")
	}
}

func TestSafeTargetC(t *testing.T) {
	if _, err := SafeTargetC(20); err == nil {
		t.Fatal("too low")
	}
	if _, err := SafeTargetC(91); err == nil {
		t.Fatal("too high")
	}
	if v, err := SafeTargetC(82); err != nil || v != 82 {
		t.Fatal(v, err)
	}
}

func TestSummarize(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	c := newClient(t, srv)
	st, _, _, err := Summarize(c)
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "Test Sauna" || st.HeaterOn || !st.DoorClosed {
		t.Fatalf("%+v", st)
	}
	if st.TempC == nil || *st.TempC != 42 {
		t.Fatalf("temp %+v", st.TempC)
	}
	if st.DeviceID != "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" {
		t.Fatalf("uuid %s", st.DeviceID)
	}
}
