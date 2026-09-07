package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/amansk/harvia-pp-cli/internal/client"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
	"github.com/amansk/harvia-pp-cli/internal/fixture"
)

func run(t *testing.T, opt *Options, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	home, envfile := opt.Home, opt.EnvFile
	cmd := newRoot(opt)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	prefixed := []string{"--home", home, "--env-file", envfile, "--no-input"}
	if opt.NoStore {
		prefixed = append(prefixed, "--no-store")
	}
	cmd.SetArgs(append(prefixed, args...))
	err := cmd.Execute()
	code = exitcode.OK
	if err != nil {
		code = handleErr(cmd, err)
	}
	return out.String(), errb.String(), code
}

func testOpt(t *testing.T, srv *fixture.Server) (*Options, string) {
	t.Helper()
	home := t.TempDir()
	return &Options{
		Home:    home,
		EnvFile: fixture.EnvFile(),
		NoInput: true,
		HTTP: &client.Client{
			EndpointsURL: srv.EndpointsURL,
			Home:         home,
			Now:          func() time.Time { return time.Unix(1_700_000_000, 0) },
		},
	}, home
}

func TestLoginDoctorStatusNoSecrets(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, home := testOpt(t, srv)

	out, errb, code := run(t, opt, "--json", "auth", "login")
	if code != 0 {
		t.Fatalf("login %d err=%s out=%s", code, errb, out)
	}
	blob := out + errb
	for _, secret := range []string{"fixture-password-not-real", "fixture-id-token", "fixture-refresh-token", "fixture-access-token"} {
		if strings.Contains(blob, secret) {
			t.Fatalf("leaked %s in %s", secret, blob)
		}
	}
	info, err := os.Stat(auth.TokenPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}

	out, errb, code = run(t, opt, "doctor")
	if code != 0 {
		t.Fatalf("doctor %d %s %s", code, out, errb)
	}
	if !strings.Contains(out, "doctor: green") {
		t.Fatalf("doctor output: %s", out)
	}

	out, errb, code = run(t, opt, "--json", "doctor")
	if code != 0 {
		t.Fatalf("doctor --json %d %s %s", code, out, errb)
	}
	var doctorEnv map[string]any
	if err := json.Unmarshal([]byte(out), &doctorEnv); err != nil {
		t.Fatal(err, out)
	}
	if doctorEnv["ok"] != true {
		t.Fatalf("doctor envelope ok want true: %s", out)
	}

	out, errb, code = run(t, opt, "--json", "auth", "status")
	if code != 0 {
		t.Fatalf("status %d %s", code, errb)
	}
	if strings.Contains(out+errb, "fixture-id-token") {
		t.Fatal("status leaked token")
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err, out)
	}
	if env["ok"] != true {
		t.Fatalf("%s", out)
	}
}

func TestDoctorJSONFailedEnvelope(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, _ := testOpt(t, srv)
	out, errb, code := run(t, opt, "--json", "doctor")
	if code == 0 {
		t.Fatalf("doctor without login should fail: %s", out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err, out, errb)
	}
	if env["ok"] != false {
		t.Fatalf("envelope ok want false, got %s", out)
	}
	if _, has := env["error"]; !has {
		t.Fatalf("expected error field: %s", out)
	}

	out, errb, code = run(t, opt, "--agent", "doctor")
	if code == 0 {
		t.Fatal("agent doctor without login should fail")
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err, out, errb)
	}
	if env["ok"] != false {
		t.Fatalf("agent envelope ok want false: %s", out)
	}
}

func TestDoctorTokenModeFail(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, home := testOpt(t, srv)
	if _, _, code := run(t, opt, "auth", "login"); code != 0 {
		t.Fatal("login")
	}
	path := auth.TokenPath(home)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := run(t, opt, "--json", "doctor")
	if code == 0 {
		t.Fatalf("doctor should fail on 0644 token: %s %s", out, errb)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err, out)
	}
	if env["ok"] != false {
		t.Fatalf("envelope ok want false: %s", out)
	}
	data, _ := env["data"].(map[string]any)
	checks, _ := data["checks"].([]any)
	var sawMode bool
	for _, raw := range checks {
		ch, _ := raw.(map[string]any)
		if ch["name"] == "token_mode" {
			sawMode = true
			if ch["ok"] != false {
				t.Fatalf("token_mode should fail: %+v", ch)
			}
		}
	}
	if !sawMode {
		t.Fatalf("missing token_mode check: %s", out)
	}
}

func TestOnRequiresYesAndOrder(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, _ := testOpt(t, srv)
	if _, _, code := run(t, opt, "auth", "login"); code != 0 {
		t.Fatal("login")
	}

	_, errb, code := run(t, opt, "--no-input", "on", "--temp", "82")
	if code != exitcode.Usage {
		t.Fatalf("on without yes: code=%d err=%s", code, errb)
	}
	for _, r := range srv.Requests() {
		if r.Path == "/devices/command" {
			t.Fatal("must not POST command without confirm")
		}
	}

	out, errb, code := run(t, opt, "--yes", "--json", "on", "--temp", "82")
	if code != 0 {
		t.Fatalf("on --yes %d %s %s", code, out, errb)
	}
	var writes []string
	for _, r := range srv.Requests() {
		switch r.Path {
		case "/devices/target", "/devices/profile", "/devices/command":
			writes = append(writes, r.Method+" "+r.Path)
		}
	}
	if len(writes) < 3 {
		t.Fatalf("writes %v", writes)
	}
	tail := writes[len(writes)-3:]
	want := []string{"PATCH /devices/target", "PATCH /devices/profile", "POST /devices/command"}
	for i := range want {
		if tail[i] != want[i] {
			t.Fatalf("order %v want %v", tail, want)
		}
	}
}

func TestAgentOnSkipsPrompt(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, _ := testOpt(t, srv)
	if _, _, code := run(t, opt, "auth", "login"); code != 0 {
		t.Fatal("login")
	}
	_, errb, code := run(t, opt, "--agent", "on")
	if code != 0 {
		t.Fatalf("agent on %d %s", code, errb)
	}
}

func TestStatusDevicesWatchHistory(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, home := testOpt(t, srv)
	if _, _, code := run(t, opt, "auth", "login"); code != 0 {
		t.Fatal("login")
	}
	out, errb, code := run(t, opt, "--json", "status")
	if code != 0 {
		t.Fatalf("status %d %s", code, errb)
	}
	if !strings.Contains(out, "Test Sauna") {
		t.Fatalf("status %s", out)
	}

	out, errb, code = run(t, opt, "--json", "devices")
	if code != 0 {
		t.Fatalf("devices %d %s", code, errb)
	}
	if !strings.Contains(out, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee") {
		t.Fatalf("devices uuid: %s", out)
	}

	out, errb, code = run(t, opt, "--json", "watch", "--count", "1", "--interval", "1")
	if code != 0 {
		t.Fatalf("watch %d %s %s", code, out, errb)
	}

	out, errb, code = run(t, opt, "--json", "history", "samples")
	if code != 0 {
		t.Fatalf("history %d %s", code, errb)
	}
	if !strings.Contains(out, "recorded_at") {
		t.Fatalf("samples %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, auth.DBFile)); err != nil {
		t.Fatal("expected sqlite file")
	}

	out, errb, code = run(t, opt, "--json", "raw", "endpoints")
	if code != 0 {
		t.Fatalf("raw %d %s", code, errb)
	}
	if !strings.Contains(out, "RestApi") {
		t.Fatalf("endpoints %s", out)
	}
}

func TestTempMidSession(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, _ := testOpt(t, srv)
	if _, _, code := run(t, opt, "auth", "login"); code != 0 {
		t.Fatal("login")
	}
	_, errb, code := run(t, opt, "--json", "temp", "85")
	if code != 0 {
		t.Fatalf("temp off %d %s", code, errb)
	}
	srv.StateOn = true
	_, errb, code = run(t, opt, "--json", "temp", "88")
	if code != 0 {
		t.Fatalf("temp on %d %s", code, errb)
	}
	var saw bool
	for _, r := range srv.Requests() {
		if r.Path == "/devices/profile" {
			saw = true
		}
	}
	if !saw {
		t.Fatal("expected Custom profile activate")
	}
}

func TestLightsFanDurationOff(t *testing.T) {
	srv := fixture.New()
	defer srv.Close()
	opt, _ := testOpt(t, srv)
	if _, _, code := run(t, opt, "auth", "login"); code != 0 {
		t.Fatal("login")
	}
	for _, args := range [][]string{
		{"lights", "on"},
		{"fan", "off"},
		{"duration", "45"},
		{"off"},
	} {
		_, errb, code := run(t, opt, append([]string{"--json"}, args...)...)
		if code != 0 {
			t.Fatalf("%v %d %s", args, code, errb)
		}
	}
}

func TestCobraTree(t *testing.T) {
	cmd := NewRoot()
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	want := []string{"auth", "doctor", "status", "on", "off", "temp", "duration", "lights", "fan", "watch", "devices", "raw", "history"}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing command %s in %v", w, names)
		}
	}
}
