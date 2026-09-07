package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/actions"
	"github.com/amansk/harvia-pp-cli/internal/client"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
	"github.com/spf13/cobra"
)

func newStatusCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show heater, temps (C), lights, fan, door, session",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			st, state, tele, err := actions.Summarize(c)
			if err != nil {
				return err
			}
			recordStatus(opt, st, state.Raw, tele.Raw, "status")
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, st)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  (%s)\n", st.Name, shortID(st.DeviceID))
			fmt.Fprintf(cmd.OutOrStdout(), "  Heater : %s  [%s]\n", onOff(st.HeaterOn), st.Profile)
			fmt.Fprintf(cmd.OutOrStdout(), "  Temp   : %sC  (target %sC)\n", fmtOpt(st.TempC), fmtOpt(st.TargetTempC))
			fmt.Fprintf(cmd.OutOrStdout(), "  Lights : %s   Fan: %s\n", onOff(st.LightOn), onOff(st.FanOn))
			door := "OPEN / check"
			if st.DoorClosed {
				door = "closed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Door   : %s\n", door)
			fmt.Fprintf(cmd.OutOrStdout(), "  Session: onTime %s min, auto-off in %s min\n", fmtOpt(st.OnTimeMin), fmtOpt(st.AutoOffMin))
			if len(st.Errors) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  ERRORS : %v\n", st.Errors)
			}
			return nil
		},
	}
}

func newOnCmd(opt *Options) *cobra.Command {
	var temp, duration int
	cmd := &cobra.Command{
		Use:   "on",
		Short: "Turn the heater on (confirm gate: real 240V)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirmOn(cmd, opt); err != nil {
				return err
			}
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			var tPtr, dPtr *int
			if cmd.Flags().Changed("temp") {
				tPtr = &temp
			}
			if cmd.Flags().Changed("duration") {
				dPtr = &duration
			}
			id, err := actions.Warmup(c, tPtr, dPtr)
			if err != nil {
				return err
			}
			if st, state, tele, err := actions.Summarize(c); err == nil {
				recordStatus(opt, st, state.Raw, tele.Raw, "cli")
			}
			return writeOut(cmd, opt, map[string]any{"heater": "on", "device_id": id, "temp_c": tPtr, "duration_min": dPtr})
		},
	}
	cmd.Flags().IntVar(&temp, "temp", 0, "Target temperature in Celsius (Custom slot)")
	cmd.Flags().IntVar(&duration, "duration", 0, "Session length in minutes")
	return cmd
}

func newOffCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Turn the heater off",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			id, err := c.DeviceID()
			if err != nil {
				return err
			}
			if err := c.Command(id, "SAUNA", "off"); err != nil {
				return err
			}
			if st, state, tele, err := actions.Summarize(c); err == nil {
				recordStatus(opt, st, state.Raw, tele.Raw, "cli")
			}
			return writeOut(cmd, opt, map[string]any{"heater": "off", "device_id": id})
		},
	}
}

func newTempCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "temp <C>",
		Short: "Set target Celsius (mid-session uses Custom slot 3)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var v int
			if _, err := fmt.Sscanf(args[0], "%d", &v); err != nil {
				return exitcode.Usagef("temp expects an integer Celsius value")
			}
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			id, mid, err := actions.SetTemp(c, v)
			if err != nil {
				return err
			}
			msg := "target temperature set"
			if mid {
				msg = "running session moved to Custom profile"
			}
			return writeOut(cmd, opt, map[string]any{"ok_temp_c": v, "device_id": id, "mid_session": mid, "message": msg})
		},
	}
}

func newDurationCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "duration <MIN>",
		Short: "Set session length in minutes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var v int
			if _, err := fmt.Sscanf(args[0], "%d", &v); err != nil || v < 1 || v > 360 {
				return exitcode.Usagef("duration expects 1-360 minutes")
			}
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			id, err := c.DeviceID()
			if err != nil {
				return err
			}
			if err := c.Command(id, "ADJUST_DURATION", v); err != nil {
				return err
			}
			return writeOut(cmd, opt, map[string]any{"duration_min": v, "device_id": id})
		},
	}
}

func newSwitchCmd(opt *Options, name, kind string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " {on|off}",
		Short: "Turn the " + name + " on or off",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state := args[0]
			if state != "on" && state != "off" {
				return exitcode.Usagef("%s expects on or off", name)
			}
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			id, err := c.DeviceID()
			if err != nil {
				return err
			}
			if err := c.Command(id, kind, state); err != nil {
				return err
			}
			return writeOut(cmd, opt, map[string]any{name: state, "device_id": id})
		},
	}
}

func newWatchCmd(opt *Options) *cobra.Command {
	var interval, count int
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Poll telemetry (use --count in tests)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval < 1 {
				return exitcode.Usagef("--interval must be >= 1")
			}
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			n := 0
			for {
				st, state, tele, err := actions.Summarize(c)
				if err != nil {
					if opt.JSON || opt.Agent {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s  error: %v\n", time.Now().Format("15:04:05"), err)
				} else {
					recordStatus(opt, st, state.Raw, tele.Raw, "watch")
					if opt.JSON || opt.Agent {
						if err := writeOut(cmd, opt, st); err != nil {
							return err
						}
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "%s  temp=%sC target=%sC heat=%s light=%s auto-off=%smin\n",
							time.Now().Format("15:04:05"), fmtOpt(st.TempC), fmtOpt(st.TargetTempC),
							onOff(st.HeaterOn), onOff(st.LightOn), fmtOpt(st.AutoOffMin))
					}
				}
				n++
				if count > 0 && n >= count {
					return nil
				}
				time.Sleep(time.Duration(interval) * time.Second)
			}
		},
	}
	cmd.Flags().IntVar(&interval, "interval", 30, "Poll interval in seconds")
	cmd.Flags().IntVar(&count, "count", 0, "Stop after N polls (0 = until interrupted)")
	return cmd
}

func newDevicesCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "devices",
		Short: "List devices and the resolved UUID (name field on Fenix)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			devs, err := c.Devices()
			if err != nil {
				return err
			}
			type row struct {
				UUID        string          `json:"uuid"`
				Name        string          `json:"name"`
				DeviceID    string          `json:"device_id_field"`
				ID          string          `json:"id_field"`
				DisplayName string          `json:"display_name,omitempty"`
				Raw         json.RawMessage `json:"raw,omitempty"`
			}
			var rows []row
			var table [][]string
			for _, d := range devs {
				r := row{
					UUID:        client.DeviceUUID(d),
					Name:        d.Name,
					DeviceID:    d.DeviceID,
					ID:          d.ID,
					DisplayName: d.DisplayName,
					Raw:         d.Raw,
				}
				rows = append(rows, r)
				table = append(table, []string{r.UUID, r.DisplayName, r.Name, r.DeviceID})
			}
			return writeHumanTable(cmd, opt, []string{"UUID", "DISPLAY", "NAME", "DEVICE_ID"}, table, rows)
		},
	}
}

func newRawCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "raw {state|telemetry|endpoints}",
		Short: "Print raw JSON for diagnostics",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			raw, err := c.RawGET(args[0])
			if err != nil {
				return err
			}
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return writeOut(cmd, opt, map[string]any{"raw": string(raw)})
			}
			return writeOut(cmd, opt, parsed)
		},
	}
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "off"
}

func fmtOpt(v *float64) string {
	if v == nil {
		return "--"
	}
	return fmt.Sprintf("%g", *v)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}
