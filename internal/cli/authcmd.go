package cli

import (
	"fmt"

	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/spf13/cobra"
)

func newAuthCmd(opt *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Login, inspect, or clear the cached Cognito session",
	}
	cmd.AddCommand(newAuthLoginCmd(opt))
	cmd.AddCommand(newAuthStatusCmd(opt))
	cmd.AddCommand(newAuthLogoutCmd(opt))
	return cmd
}

func newAuthLoginCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Exchange MyHarvia email/password for tokens (never printed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			if err := auth.EnsureHome(home); err != nil {
				return err
			}
			creds, err := auth.ResolveCreds(opt.envFiles(home), opt.EnvFile != "")
			if err != nil {
				return err
			}
			creds, err = promptCreds(opt, creds)
			if err != nil {
				return err
			}
			c, err := opt.newClient()
			if err != nil {
				return err
			}
			c.Home = home
			c.Username = creds.Username
			c.Password = creds.Password
			if _, err := c.Login(); err != nil {
				return err
			}
			st, err := auth.Inspect(home)
			if err != nil {
				return err
			}
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, st)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "logged in as %s\ntokens stored at %s (mode 0600)\n", st.Username, st.Path)
			return nil
		},
	}
}

func newAuthStatusCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show session presence (no token values)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			st, err := auth.Inspect(home)
			if err != nil {
				return err
			}
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, st)
			}
			if !st.Present {
				fmt.Fprintln(cmd.OutOrStdout(), "not logged in")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "user        %s\nid_token    %t\nrefresh     %t\nexpires_in  %.0fs\npath        %s\nmode        %s\n",
				st.Username, st.HasIDToken, st.HasRefreshToken, st.ExpiresInS, st.Path, st.Mode)
			return nil
		},
	}
}

func newAuthLogoutCmd(opt *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the cached token file",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opt.ResolveHome()
			if err != nil {
				return err
			}
			if err := auth.DeleteTokens(home); err != nil {
				return err
			}
			return writeOut(cmd, opt, map[string]any{"logged_out": true, "path": auth.TokenPath(home)})
		},
	}
}
