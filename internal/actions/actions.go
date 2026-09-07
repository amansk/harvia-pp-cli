// Package actions composes Harvia primitives into heater operations.
package actions

import (
	"math"

	"github.com/amansk/harvia-pp-cli/internal/client"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
)

// Custom profile slot (index 3). A running session follows this slot's
// target; a bare PATCH /devices/target is ignored mid-session.
const CustomProfileSlot = 3

const (
	MinTempC = 32
	MaxTempC = 90
)

var profileNames = map[string]string{
	"0": "Mild",
	"1": "Cozy",
	"2": "Hot",
	"3": "Custom",
}

// SafeTargetC clamps a temperature to the unit's documented range.
// Non-finite input returns an error rather than inventing a value.
func SafeTargetC(tempC int) (int, error) {
	if tempC < MinTempC || tempC > MaxTempC {
		return 0, exitcode.Usagef("temperature %dC is outside the unit range %d-%dC", tempC, MinTempC, MaxTempC)
	}
	return tempC, nil
}

// ProfileLabel is a human name for the active program.
func ProfileLabel(st client.State) string {
	active := st.ActiveProfile
	if p, ok := st.Profiles[active]; ok && p.Name != "" {
		return p.Name
	}
	if name, ok := profileNames[active]; ok {
		return name
	}
	if active == "" {
		return ""
	}
	return "profile " + active
}

func customHumidity(st client.State) int {
	if p, ok := st.Profiles["3"]; ok && p.TargetHum != nil && !math.IsNaN(*p.TargetHum) {
		return int(math.Round(*p.TargetHum))
	}
	return 0
}

// WriteCustomTemp writes temp into slot 3 and activates it (trap 2).
func WriteCustomTemp(c *client.Client, deviceID string, tempC int) error {
	temp, err := SafeTargetC(tempC)
	if err != nil {
		return err
	}
	st, err := c.State(deviceID)
	if err != nil {
		return err
	}
	hum := customHumidity(st)
	slot := CustomProfileSlot
	if err := c.SetTarget(deviceID, &temp, &hum, &slot); err != nil {
		return err
	}
	return c.SetProfile(deviceID, CustomProfileSlot)
}

// Warmup turns the heater on. When tempC is set, the Custom-slot path is
// used so a subsequent session honors the requested temperature.
func Warmup(c *client.Client, tempC, durationMin *int) (string, error) {
	deviceID, err := c.DeviceID()
	if err != nil {
		return "", err
	}
	if tempC != nil {
		if err := WriteCustomTemp(c, deviceID, *tempC); err != nil {
			return "", err
		}
	}
	if durationMin != nil {
		if *durationMin < 1 || *durationMin > 360 {
			return "", exitcode.Usagef("duration must be 1-360 minutes")
		}
		if err := c.Command(deviceID, "ADJUST_DURATION", *durationMin); err != nil {
			return "", err
		}
	}
	if err := c.Command(deviceID, "SAUNA", "on"); err != nil {
		return "", err
	}
	return deviceID, nil
}

// SetTemp applies trap 2 when the heater is on; otherwise a bare target patch.
func SetTemp(c *client.Client, tempC int) (string, bool, error) {
	temp, err := SafeTargetC(tempC)
	if err != nil {
		return "", false, err
	}
	deviceID, err := c.DeviceID()
	if err != nil {
		return "", false, err
	}
	st, err := c.State(deviceID)
	if err != nil {
		return "", false, err
	}
	if st.HeaterOn {
		if err := WriteCustomTemp(c, deviceID, temp); err != nil {
			return "", true, err
		}
		return deviceID, true, nil
	}
	if err := c.SetTarget(deviceID, &temp, nil, nil); err != nil {
		return "", false, err
	}
	return deviceID, false, nil
}

// Status is the compact display / JSON view.
type Status struct {
	Name        string   `json:"name"`
	DeviceID    string   `json:"device_id"`
	HeaterOn    bool     `json:"heater_on"`
	Profile     string   `json:"profile"`
	TempC       *float64 `json:"temp_c"`
	TargetTempC *float64 `json:"target_temp_c"`
	LightOn     bool     `json:"light_on"`
	FanOn       bool     `json:"fan_on"`
	DoorClosed  bool     `json:"door_closed"`
	AutoOffMin  *float64 `json:"auto_off_min"`
	OnTimeMin   *float64 `json:"on_time_min"`
	Errors      []string `json:"errors,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

// Summarize reads state + telemetry.
func Summarize(c *client.Client) (Status, client.State, client.Telemetry, error) {
	deviceID, err := c.DeviceID()
	if err != nil {
		return Status{}, client.State{}, client.Telemetry{}, err
	}
	st, err := c.State(deviceID)
	if err != nil {
		return Status{}, client.State{}, client.Telemetry{}, err
	}
	tele, err := c.Telemetry(deviceID)
	if err != nil {
		return Status{}, st, client.Telemetry{}, err
	}
	target := tele.TargetTemp
	if target == nil {
		target = st.TargetTemp
	}
	doorClosed := tele.DoorSafetyState != nil && *tele.DoorSafetyState == 1
	name := st.DisplayName
	if name == "" {
		name = "Sauna"
	}
	return Status{
		Name:        name,
		DeviceID:    deviceID,
		HeaterOn:    st.HeaterOn,
		Profile:     ProfileLabel(st),
		TempC:       tele.Temp,
		TargetTempC: target,
		LightOn:     tele.LightOn,
		FanOn:       tele.FanOn,
		DoorClosed:  doorClosed,
		AutoOffMin:  tele.AutoOffTime,
		OnTimeMin:   tele.OnTime,
		Errors:      st.Errors,
		Warnings:    st.Warnings,
	}, st, tele, nil
}
