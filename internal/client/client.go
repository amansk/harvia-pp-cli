// Package client is a thin MyHarvia 2 / Fenix REST client.
//
// Wire shapes were verified against sauna-cloud cli/harvia and public HA
// clients. Hosts rotate; the endpoint map is process-local only.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
)

const (
	DefaultEndpointsURL = "https://api.harvia.io/endpoints"
	HTTPTimeout         = 20 * time.Second
	CabinID             = "C1"
)

// Client talks to Harvia REST after endpoint discovery + Cognito login.
type Client struct {
	HTTP         *http.Client
	EndpointsURL string
	Home         string
	Username     string
	Password     string
	DeviceHint   string // optional --device
	Now          func() time.Time

	endpoints *EndpointMap
	tokens    *auth.Tokens
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: HTTPTimeout}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) endpointsURL() string {
	if c.EndpointsURL != "" {
		return c.EndpointsURL
	}
	if u := os.Getenv("HARVIA_ENDPOINTS_URL"); u != "" {
		return u
	}
	return DefaultEndpointsURL
}

func (c *Client) do(method, rawURL, bearer string, body any) (json.RawMessage, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, rdr)
	if err != nil {
		return nil, 0, exitcode.APIf("%s %s: %v", method, rawURL, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, 0, exitcode.Transientf("%s %s: %v", method, redactURL(rawURL), err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		secrets := append([]string{c.Password}, tokenValues(c.tokens)...)
		detail := auth.RedactSecrets(string(raw), secrets...)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		code := exitcode.API
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			code = exitcode.Auth
		}
		if resp.StatusCode >= 500 {
			code = exitcode.Transient
		}
		return raw, resp.StatusCode, exitcode.Wrap(code, fmt.Errorf("%s %s -> HTTP %d: %s", method, redactURL(rawURL), resp.StatusCode, detail))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`), resp.StatusCode, nil
	}
	if !json.Valid(raw) {
		return raw, resp.StatusCode, exitcode.APIf("%s %s: response was not JSON", method, redactURL(rawURL))
	}
	return json.RawMessage(raw), resp.StatusCode, nil
}

func tokenValues(t *auth.Tokens) []string {
	if t == nil {
		return nil
	}
	return []string{t.IDToken, t.AccessToken, t.RefreshToken}
}

func redactURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	parsed.User = nil
	return parsed.String()
}

// Endpoints fetches (and memoizes) the rotating REST map.
func (c *Client) Endpoints() (EndpointMap, error) {
	if c.endpoints != nil {
		return *c.endpoints, nil
	}
	raw, _, err := c.do(http.MethodGet, c.endpointsURL(), "", nil)
	if err != nil {
		return EndpointMap{}, err
	}
	var wrap struct {
		Endpoints EndpointMap `json:"endpoints"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return EndpointMap{}, exitcode.APIf("endpoints: %v", err)
	}
	if wrap.Endpoints.RestAPI == nil {
		return EndpointMap{}, exitcode.APIf("endpoint discovery returned no RestApi")
	}
	c.endpoints = &wrap.Endpoints
	return wrap.Endpoints, nil
}

func (c *Client) base(service string) (string, error) {
	ep, err := c.Endpoints()
	if err != nil {
		return "", err
	}
	svc, ok := ep.RestAPI[service]
	if !ok || svc.HTTPS == "" {
		return "", exitcode.APIf("endpoint map has no RestApi.%s.https", service)
	}
	return strings.TrimRight(svc.HTTPS, "/"), nil
}

// Login exchanges username/password for Cognito tokens and writes token.json.
func (c *Client) Login() (auth.Tokens, error) {
	if c.Username == "" || c.Password == "" {
		return auth.Tokens{}, exitcode.Authf("missing credentials: set HARVIA_USERNAME and HARVIA_PASSWORD (env, --env-file, or ~/.config/harvia/env)")
	}
	base, err := c.base("generics")
	if err != nil {
		return auth.Tokens{}, err
	}
	raw, _, err := c.do(http.MethodPost, base+"/auth/token", "", map[string]string{
		"username": c.Username,
		"password": c.Password,
	})
	if err != nil {
		return auth.Tokens{}, exitcode.Authf("login failed (wrong password, or Apple/Google SSO with no MyHarvia password set): %v", err)
	}
	var tok Tokens
	if err := json.Unmarshal(raw, &tok); err != nil {
		return auth.Tokens{}, exitcode.APIf("login: %v", err)
	}
	if tok.IDToken == "" {
		return auth.Tokens{}, exitcode.Authf("auth/token returned no idToken")
	}
	stored := auth.Tokens{
		IDToken:      tok.IDToken,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresIn:    tok.ExpiresIn,
		FetchedAt:    float64(c.now().Unix()),
		Username:     c.Username,
	}
	if stored.ExpiresIn == 0 {
		stored.ExpiresIn = 3600
	}
	if err := auth.SaveTokens(c.Home, stored); err != nil {
		return auth.Tokens{}, err
	}
	c.tokens = &stored
	return stored, nil
}

func (c *Client) refresh(refreshToken string) (*auth.Tokens, error) {
	base, err := c.base("generics")
	if err != nil {
		return nil, err
	}
	raw, _, err := c.do(http.MethodPost, base+"/auth/refresh", "", map[string]string{
		"refreshToken": refreshToken,
		"email":        c.Username,
	})
	if err != nil {
		return nil, err
	}
	var tok Tokens
	if err := json.Unmarshal(raw, &tok); err != nil || tok.IDToken == "" {
		return nil, exitcode.Authf("refresh did not return idToken")
	}
	// Trap: refresh does not rotate. Keep the old refresh token.
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	stored := auth.Tokens{
		IDToken:      tok.IDToken,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresIn:    tok.ExpiresIn,
		FetchedAt:    float64(c.now().Unix()),
		Username:     c.Username,
	}
	if stored.ExpiresIn == 0 {
		stored.ExpiresIn = 3600
	}
	if err := auth.SaveTokens(c.Home, stored); err != nil {
		return nil, err
	}
	c.tokens = &stored
	return &stored, nil
}

// IDToken returns a usable bearer, refreshing or re-logging in as needed.
func (c *Client) IDToken(force bool) (string, error) {
	if !force && auth.TokenValid(c.tokens, c.now(), c.Username) {
		return c.tokens.IDToken, nil
	}
	if !force {
		cached, err := auth.LoadTokens(c.Home)
		if err != nil {
			return "", err
		}
		if auth.TokenValid(cached, c.now(), c.Username) {
			c.tokens = cached
			if c.Username == "" {
				c.Username = cached.Username
			}
			return cached.IDToken, nil
		}
		if cached != nil && cached.RefreshToken != "" && (c.Username == "" || cached.Username == c.Username || cached.Username == "") {
			if c.Username == "" {
				c.Username = cached.Username
			}
			refreshed, err := c.refresh(cached.RefreshToken)
			if err == nil && refreshed != nil {
				return refreshed.IDToken, nil
			}
		}
	}
	if c.Username == "" || c.Password == "" {
		return "", exitcode.Authf("session expired; re-run auth login (or set HARVIA_USERNAME / HARVIA_PASSWORD)")
	}
	stored, err := c.Login()
	if err != nil {
		return "", err
	}
	return stored.IDToken, nil
}

func (c *Client) authed(method, rawURL string, body any) (json.RawMessage, error) {
	tok, err := c.IDToken(false)
	if err != nil {
		return nil, err
	}
	raw, status, err := c.do(method, rawURL, tok, body)
	if err != nil && status == 401 {
		tok, recErr := c.recoverAfter401()
		if recErr != nil {
			return nil, recErr
		}
		raw, _, err = c.do(method, rawURL, tok, body)
	}
	return raw, err
}

func (c *Client) invalidateIDToken() {
	if c.tokens != nil {
		c.tokens.IDToken = ""
	}
}

func (c *Client) refreshMaterial() (refreshToken, username string) {
	if c.tokens != nil {
		refreshToken = c.tokens.RefreshToken
		username = c.tokens.Username
	}
	if refreshToken != "" {
		return refreshToken, username
	}
	cached, err := auth.LoadTokens(c.Home)
	if err != nil || cached == nil {
		return "", username
	}
	return cached.RefreshToken, firstNonEmpty(username, cached.Username)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// recoverAfter401 drops the cached idToken, tries refresh, then re-login.
// Token-only sessions can recover via refresh. Failures return the
// refresh/re-login error, not the original 401.
func (c *Client) recoverAfter401() (string, error) {
	c.invalidateIDToken()
	refreshTok, user := c.refreshMaterial()
	if c.Username == "" {
		c.Username = user
	}

	var refreshErr error
	if refreshTok != "" {
		refreshed, err := c.refresh(refreshTok)
		if err == nil && refreshed != nil && refreshed.IDToken != "" {
			return refreshed.IDToken, nil
		}
		refreshErr = err
		if refreshErr == nil {
			refreshErr = exitcode.Authf("refresh did not return idToken")
		}
	}

	if c.Username == "" || c.Password == "" {
		if refreshErr != nil {
			return "", refreshErr
		}
		return "", exitcode.Authf("session expired; re-run auth login (or set HARVIA_USERNAME / HARVIA_PASSWORD)")
	}
	stored, err := c.Login()
	if err != nil {
		return "", err
	}
	return stored.IDToken, nil
}

// Devices lists account devices (paginated).
func (c *Client) Devices() ([]Device, error) {
	base, err := c.base("device")
	if err != nil {
		return nil, err
	}
	var out []Device
	next := ""
	for {
		u := base + "/devices?maxResults=100"
		if next != "" {
			u += "&nextToken=" + url.QueryEscape(next)
		}
		raw, err := c.authed(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Devices   []json.RawMessage `json:"devices"`
			NextToken string            `json:"nextToken"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, exitcode.APIf("devices: %v", err)
		}
		for _, item := range page.Devices {
			var d Device
			_ = json.Unmarshal(item, &d)
			d.Raw = item
			out = append(out, d)
		}
		if page.NextToken == "" {
			return out, nil
		}
		next = page.NextToken
	}
}

// DeviceID resolves the heater UUID (trap 1).
func (c *Client) DeviceID() (string, error) {
	if c.DeviceHint != "" {
		return c.DeviceHint, nil
	}
	devs, err := c.Devices()
	if err != nil {
		return "", err
	}
	if len(devs) == 0 {
		return "", exitcode.NotFoundf("account has no devices")
	}
	id := DeviceUUID(devs[0])
	if id == "" {
		return "", exitcode.APIf("device entry has no usable identifier (expected UUID in name)")
	}
	return id, nil
}

// State fetches GET /devices/state.
func (c *Client) State(deviceID string) (State, error) {
	base, err := c.base("device")
	if err != nil {
		return State{}, err
	}
	u := fmt.Sprintf("%s/devices/state?deviceId=%s&subId=%s", base, url.QueryEscape(deviceID), CabinID)
	raw, err := c.authed(http.MethodGet, u, nil)
	if err != nil {
		return State{}, err
	}
	return parseState(raw), nil
}

// Telemetry fetches GET /data/latest-data.
func (c *Client) Telemetry(deviceID string) (Telemetry, error) {
	base, err := c.base("data")
	if err != nil {
		return Telemetry{}, err
	}
	u := fmt.Sprintf("%s/data/latest-data?deviceId=%s&cabinId=%s", base, url.QueryEscape(deviceID), CabinID)
	raw, err := c.authed(http.MethodGet, u, nil)
	if err != nil {
		return Telemetry{}, err
	}
	return parseTelemetry(raw), nil
}

// Command posts an on/off (or duration) command.
func (c *Client) Command(deviceID, typ string, state any) error {
	base, err := c.base("device")
	if err != nil {
		return err
	}
	_, err = c.authed(http.MethodPost, base+"/devices/command", map[string]any{
		"deviceId": deviceID,
		"cabin":    map[string]string{"id": CabinID},
		"command":  map[string]any{"type": typ, "state": state},
	})
	return err
}

// SetTarget patches temperature/humidity, optionally into a profile slot.
func (c *Client) SetTarget(deviceID string, temperature, humidity *int, profile *int) error {
	if temperature == nil && humidity == nil {
		return exitcode.Usagef("set target needs temperature and/or humidity")
	}
	base, err := c.base("device")
	if err != nil {
		return err
	}
	body := map[string]any{
		"deviceId": deviceID,
		"cabin":    map[string]string{"id": CabinID},
	}
	if temperature != nil {
		body["temperature"] = *temperature
	}
	if humidity != nil {
		body["humidity"] = *humidity
	}
	if profile != nil {
		body["profile"] = fmt.Sprintf("%d", *profile)
	}
	_, err = c.authed(http.MethodPatch, base+"/devices/target", body)
	return err
}

// SetProfile activates a program slot (0-3).
func (c *Client) SetProfile(deviceID string, profile int) error {
	base, err := c.base("device")
	if err != nil {
		return err
	}
	_, err = c.authed(http.MethodPatch, base+"/devices/profile", map[string]any{
		"deviceId": deviceID,
		"profile":  fmt.Sprintf("%d", profile),
	})
	return err
}

// RawGET is a diagnostics helper (authenticated unless url is the endpoints URL).
func (c *Client) RawGET(kind string) (json.RawMessage, error) {
	switch kind {
	case "endpoints":
		_, err := c.Endpoints()
		if err != nil {
			return nil, err
		}
		raw, _, err := c.do(http.MethodGet, c.endpointsURL(), "", nil)
		return raw, err
	case "state":
		id, err := c.DeviceID()
		if err != nil {
			return nil, err
		}
		st, err := c.State(id)
		if err != nil {
			return nil, err
		}
		return st.Raw, nil
	case "telemetry":
		id, err := c.DeviceID()
		if err != nil {
			return nil, err
		}
		t, err := c.Telemetry(id)
		if err != nil {
			return nil, err
		}
		return t.Raw, nil
	default:
		return nil, exitcode.Usagef("raw expects state, telemetry, or endpoints")
	}
}
