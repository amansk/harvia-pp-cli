package cli

import (
	"fmt"
	"os"

	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
	"github.com/spf13/cobra"
)

func newDoctorCmd(opt *Options) *cobra.Command {
	var live bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local config, tokens, and SQLite (read-only --live)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			type check struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail,omitempty"`
			}
			var checks []check
			add := func(name string, ok bool, detail string) {
				checks = append(checks, check{Name: name, OK: ok, Detail: detail})
			}

			if err := os.MkdirAll(home, 0o700); err != nil {
				add("home", false, err.Error())
			} else {
				add("home", true, home)
			}

			st, err := auth.Inspect(home)
			if err != nil {
				add("token", false, err.Error())
			} else if !st.Present {
				add("token", false, "missing token.json; run auth login")
			} else {
				detail := fmt.Sprintf("user=%s id_token=%t refresh=%t", st.Username, st.HasIDToken, st.HasRefreshToken)
				if st.Mode != "" && st.Mode != "0600" {
					detail += " (warn: mode " + st.Mode + ", want 0600)"
				}
				add("token", true, detail)
			}

			if opt.NoStore {
				add("sqlite", true, "skipped (--no-store)")
			} else {
				db, err := opt.openStore()
				if err != nil {
					add("sqlite", false, err.Error())
				} else if db == nil {
					add("sqlite", true, "skipped")
				} else {
					n, _ := db.SampleCount()
					_ = db.Close()
					add("sqlite", true, fmt.Sprintf("%s samples=%d", db.Path, n))
				}
			}

			if live {
				c, err := opt.newClient()
				if err != nil {
					add("live", false, err.Error())
				} else {
					if _, err := c.Endpoints(); err != nil {
						add("live_endpoints", false, err.Error())
					} else {
						add("live_endpoints", true, "ok")
					}
					devs, err := c.Devices()
					if err != nil {
						add("live_devices", false, err.Error())
					} else {
						add("live_devices", true, fmt.Sprintf("count=%d", len(devs)))
					}
				}
			}

			ok := true
			for _, ch := range checks {
				if !ch.OK {
					ok = false
				}
			}
			payload := map[string]any{"ok": ok, "checks": checks}
			if opt.JSON || opt.Agent {
				if err := writeOut(cmd, opt, payload); err != nil {
					return err
				}
			} else {
				for _, ch := range checks {
					mark := "ok"
					if !ch.OK {
						mark = "FAIL"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-16s %s  %s\n", ch.Name, mark, ch.Detail)
				}
				if ok {
					fmt.Fprintln(cmd.OutOrStdout(), "doctor: green")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "doctor: FAIL")
				}
			}
			if !ok {
				return &exitcode.Error{Code: exitcode.API, Err: fmt.Errorf("doctor failed"), Silent: true}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&live, "live", false, "Read-only ping (endpoints + devices). Never turns the heater on")
	return cmd
}
