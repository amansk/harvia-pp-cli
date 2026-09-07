// Package cli is the cobra command tree for harvia-pp-cli.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/actions"
	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/amansk/harvia-pp-cli/internal/client"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
	"github.com/amansk/harvia-pp-cli/internal/output"
	"github.com/amansk/harvia-pp-cli/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// version is overridden at build time via -ldflags "-X .../internal/cli.version=...".
var version = "0.1.0"

// Options are global flags.
type Options struct {
	JSON    bool
	Agent   bool
	Quiet   bool
	NoColor bool
	NoInput bool
	Yes     bool
	NoStore bool
	Home    string
	EnvFile string
	Device  string
	HTTP    *client.Client // tests inject a preconfigured client
}

func (o Options) Mode() output.Mode {
	return output.Mode{JSON: o.JSON || o.Agent, Agent: o.Agent, Quiet: o.Quiet, NoColor: o.NoColor || o.Agent}
}

func (o Options) ResolveHome() (string, error) {
	return auth.HomeDir(o.Home)
}

func (o Options) envFiles(home string) []string {
	var files []string
	if o.EnvFile != "" {
		files = append(files, o.EnvFile)
	}
	files = append(files, auth.DefaultEnvCandidates(home)...)
	return files
}

func (o *Options) newClient() (*client.Client, error) {
	if o.HTTP != nil {
		if o.HTTP.Home == "" {
			home, err := o.ResolveHome()
			if err != nil {
				return nil, err
			}
			o.HTTP.Home = home
		}
		if o.Device != "" {
			o.HTTP.DeviceHint = o.Device
		}
		return o.HTTP, nil
	}
	home, err := o.ResolveHome()
	if err != nil {
		return nil, err
	}
	creds, err := auth.ResolveCreds(o.envFiles(home), o.EnvFile != "")
	if err != nil {
		return nil, err
	}
	c := &client.Client{
		Home:       home,
		Username:   creds.Username,
		Password:   creds.Password,
		DeviceHint: o.Device,
	}
	if cached, _ := auth.LoadTokens(home); cached != nil && c.Username == "" {
		c.Username = cached.Username
	}
	return c, nil
}

func (o Options) openStore() (*store.DB, error) {
	if o.NoStore {
		return nil, nil
	}
	home, err := o.ResolveHome()
	if err != nil {
		return nil, err
	}
	return store.Open(home)
}

func recordStatus(opt *Options, st actions.Status, rawState, rawTele []byte, source string) {
	db, err := opt.openStore()
	if err != nil || db == nil {
		return
	}
	defer func() { _ = db.Close() }()
	_ = db.RecordSample(st, rawState, rawTele, source, time.Time{})
}

// NewRoot builds the command tree.
func NewRoot() *cobra.Command {
	return newRoot(nil)
}

func newRoot(opt *Options) *cobra.Command {
	if opt == nil {
		opt = &Options{}
	}
	cmd := &cobra.Command{
		Use:           "harvia-pp-cli",
		Short:         "Agent-native Harvia MyHarvia 2 / Fenix CLI",
		Long:          "Cognito-auth heater control + optional SQLite telemetry. Never prints secrets. on requires confirmation (real 240V).",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if opt.Agent {
				opt.JSON = true
				opt.NoColor = true
				opt.NoInput = true
				opt.Yes = true
			}
		},
	}
	cmd.PersistentFlags().BoolVar(&opt.JSON, "json", false, "Emit machine-readable JSON")
	cmd.PersistentFlags().BoolVar(&opt.Agent, "agent", false, "JSON + compact + no prompts + no color + yes")
	cmd.PersistentFlags().BoolVar(&opt.Quiet, "quiet", false, "Suppress human chatter")
	cmd.PersistentFlags().BoolVar(&opt.NoColor, "no-color", false, "Disable color")
	cmd.PersistentFlags().BoolVar(&opt.NoInput, "no-input", false, "Never read a TTY prompt")
	cmd.PersistentFlags().BoolVar(&opt.Yes, "yes", false, "Confirm destructive/heater-on actions")
	cmd.PersistentFlags().BoolVar(&opt.NoStore, "no-store", false, "Do not write SQLite samples")
	cmd.PersistentFlags().StringVar(&opt.Home, "home", "", "Override state dir ($HARVIA_PP_HOME or ~/.config/harvia)")
	cmd.PersistentFlags().StringVar(&opt.EnvFile, "env-file", "", "Load HARVIA_USERNAME / HARVIA_PASSWORD from this file")
	cmd.PersistentFlags().StringVar(&opt.Device, "device", "", "Device UUID (default: first account device)")

	cmd.AddCommand(newAuthCmd(opt))
	cmd.AddCommand(newDoctorCmd(opt))
	cmd.AddCommand(newStatusCmd(opt))
	cmd.AddCommand(newOnCmd(opt))
	cmd.AddCommand(newOffCmd(opt))
	cmd.AddCommand(newTempCmd(opt))
	cmd.AddCommand(newDurationCmd(opt))
	cmd.AddCommand(newSwitchCmd(opt, "lights", "LIGHTS"))
	cmd.AddCommand(newSwitchCmd(opt, "fan", "FAN"))
	cmd.AddCommand(newWatchCmd(opt))
	cmd.AddCommand(newDevicesCmd(opt))
	cmd.AddCommand(newRawCmd(opt))
	cmd.AddCommand(newHistoryCmd(opt))
	return cmd
}

// Execute runs the root command and maps errors to exit codes.
func Execute() int {
	cmd := NewRoot()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	if err := cmd.Execute(); err != nil {
		return handleErr(cmd, err)
	}
	return exitcode.OK
}

func handleErr(cmd *cobra.Command, err error) int {
	code := exitcode.Usage
	var ex *exitcode.Error
	if exitcode.As(err, &ex) {
		code = ex.Code
		if ex.Silent {
			return code
		}
	}
	if cmd != nil {
		jsonFlag, _ := cmd.Root().PersistentFlags().GetBool("json")
		agentFlag, _ := cmd.Root().PersistentFlags().GetBool("agent")
		mode := output.Mode{JSON: jsonFlag || agentFlag, Agent: agentFlag}
		_ = mode.EncodeError(cmd.ErrOrStderr(), err)
		return code
	}
	fmt.Fprintln(os.Stderr, err.Error())
	return code
}

func writeOut(cmd *cobra.Command, opt *Options, data any) error {
	return opt.Mode().Encode(cmd.OutOrStdout(), data)
}

func writeOutStatus(cmd *cobra.Command, opt *Options, ok bool, data any, errMsg string) error {
	return opt.Mode().EncodeStatus(cmd.OutOrStdout(), ok, data, errMsg)
}

func writeHumanTable(cmd *cobra.Command, opt *Options, headers []string, rows [][]string, jsonData any) error {
	if opt.JSON || opt.Agent {
		return writeOut(cmd, opt, jsonData)
	}
	return output.Table(cmd.OutOrStdout(), headers, rows)
}

func confirmOn(cmd *cobra.Command, opt *Options) error {
	if opt.Yes || opt.Agent {
		return nil
	}
	if opt.NoInput || !isTTY(os.Stdin) {
		return exitcode.Usagef("refusing to power a 240V heater without --yes (or --agent)")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "This powers a real 240V sauna heater. Continue? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return exitcode.Usagef("confirmation failed: %v", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		return exitcode.Usagef("aborted")
	}
	return nil
}

func promptCreds(opt *Options, creds auth.Creds) (auth.Creds, error) {
	if creds.Username != "" && creds.Password != "" {
		return creds, nil
	}
	if opt.NoInput || opt.Agent {
		return creds, exitcode.Authf("missing HARVIA_USERNAME / HARVIA_PASSWORD (use --env-file or env; --no-input refuses prompts)")
	}
	if !isTTY(os.Stdin) {
		return creds, exitcode.Authf("missing credentials and stdin is not a TTY")
	}
	in := bufio.NewReader(os.Stdin)
	if creds.Username == "" {
		fmt.Fprint(os.Stderr, "MyHarvia email: ")
		line, err := in.ReadString('\n')
		if err != nil {
			return creds, exitcode.Authf("read username: %v", err)
		}
		creds.Username = strings.TrimSpace(line)
	}
	if creds.Password == "" {
		fmt.Fprint(os.Stderr, "MyHarvia password: ")
		b, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return creds, exitcode.Authf("read password: %v", err)
		}
		creds.Password = string(b)
	}
	if creds.Username == "" || creds.Password == "" {
		return creds, exitcode.Authf("username and password are required")
	}
	return creds, nil
}

// isTTY reports whether f is an interactive terminal. os.ModeCharDevice is
// not enough: /dev/null is a character device, so `on </dev/null` would
// otherwise print a prompt to nobody.
func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
