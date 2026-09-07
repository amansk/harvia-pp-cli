package client

import (
	"encoding/json"
	"regexp"
	"strings"
)

// EndpointMap is endpoints.RestApi from GET /endpoints.
type EndpointMap struct {
	RestAPI map[string]struct {
		HTTPS string `json:"https"`
	} `json:"RestApi"`
}

// Tokens is a login or refresh response. Refresh omits refreshToken.
type Tokens struct {
	AccessToken  string  `json:"accessToken"`
	IDToken      string  `json:"idToken"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresIn    float64 `json:"expiresIn"`
}

// Device is one row from GET /devices. On Fenix, the UUID lives in Name.
type Device struct {
	DeviceID    string          `json:"deviceId"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	Raw         json.RawMessage `json:"-"`
}

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// LooksLikeUUID reports a canonical 8-4-4-4-12 hex id.
func LooksLikeUUID(s string) bool {
	return uuidRE.MatchString(strings.TrimSpace(s))
}

// DeviceUUID applies trap 1: Fenix GET /devices puts the UUID in name, not
// deviceId. Prefer a UUID-shaped name; otherwise fall back through deviceId,
// id, then name.
func DeviceUUID(d Device) string {
	if LooksLikeUUID(d.Name) {
		return d.Name
	}
	if d.DeviceID != "" {
		return d.DeviceID
	}
	if d.ID != "" {
		return d.ID
	}
	return d.Name
}

// State is a defensive decode of GET /devices/state.
type State struct {
	DisplayName   string
	HeaterOn      bool
	ActiveProfile string
	TargetTemp    *float64
	TargetHum     *float64
	Profiles      map[string]Profile
	Errors        []string
	Warnings      []string
	Raw           json.RawMessage
}

// Profile is one program slot (0 Mild, 1 Cozy, 2 Hot, 3 Custom).
type Profile struct {
	Name       string
	TargetTemp *float64
	TargetHum  *float64
}

// Telemetry is a defensive decode of GET /data/latest-data.
type Telemetry struct {
	Temp            *float64
	TargetTemp      *float64
	Hum             *float64
	LightOn         bool
	FanOn           bool
	HeatOn          bool
	DoorSafetyState *float64
	AutoOffTime     *float64
	OnTime          *float64
	Raw             json.RawMessage
}

func parseState(raw json.RawMessage) State {
	var wrap struct {
		State map[string]any `json:"state"`
	}
	_ = json.Unmarshal(raw, &wrap)
	src := wrap.State
	if src == nil {
		_ = json.Unmarshal(raw, &src)
	}
	if src == nil {
		return State{Raw: raw}
	}
	st := State{Raw: raw, Profiles: map[string]Profile{}}
	st.DisplayName, _ = src["displayName"].(string)
	if h, ok := src["heater"].(map[string]any); ok {
		st.HeaterOn = asBool(h["on"])
	}
	st.ActiveProfile = asString(src["activeProfile"])
	st.TargetTemp = asFloat(src["targetTemp"])
	st.TargetHum = asFloat(src["targetHum"])
	if p, ok := src["profiles"].(map[string]any); ok {
		for k, v := range p {
			pm, _ := v.(map[string]any)
			if pm == nil {
				continue
			}
			name, _ := pm["name"].(string)
			st.Profiles[k] = Profile{
				Name:       name,
				TargetTemp: asFloat(pm["targetTemp"]),
				TargetHum:  asFloat(pm["targetHum"]),
			}
		}
	}
	st.Errors = asStringSlice(src["errors"])
	st.Warnings = asStringSlice(src["warnings"])
	return st
}

func parseTelemetry(raw json.RawMessage) Telemetry {
	var wrap struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(raw, &wrap)
	src := wrap.Data
	if src == nil {
		_ = json.Unmarshal(raw, &src)
	}
	if src == nil {
		return Telemetry{Raw: raw}
	}
	return Telemetry{
		Raw:             raw,
		Temp:            asFloat(src["temp"]),
		TargetTemp:      asFloat(src["targetTemp"]),
		Hum:             asFloat(src["hum"]),
		LightOn:         asBool(src["lightOn"]),
		FanOn:           asBool(src["fanOn"]),
		HeatOn:          asBool(src["heatOn"]),
		DoorSafetyState: asFloat(src["doorSafetyState"]),
		AutoOffTime:     asFloat(src["autoOffTime"]),
		OnTime:          asFloat(src["onTime"]),
	}
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case json.Number:
		n, _ := t.Float64()
		return n != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "1" || s == "true" || s == "on"
	}
	return false
}

func asFloat(v any) *float64 {
	switch t := v.(type) {
	case float64:
		x := t
		return &x
	case json.Number:
		n, err := t.Float64()
		if err != nil {
			return nil
		}
		return &n
	}
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int(t)) {
			return strings.TrimSuffix(strings.TrimRight(jsonNumber(t), "0"), ".")
		}
	}
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func asStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		s := asString(item)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
