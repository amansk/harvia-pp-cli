package cli

import (
	"fmt"

	"github.com/amansk/harvia-pp-cli/internal/exitcode"
	"github.com/spf13/cobra"
)

func newHistoryCmd(opt *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Offline SQLite samples, sessions, and hours-on",
	}
	cmd.AddCommand(newHistorySamplesCmd(opt))
	cmd.AddCommand(newHistorySessionsCmd(opt))
	cmd.AddCommand(newHistoryHoursCmd(opt))
	return cmd
}

func newHistorySamplesCmd(opt *Options) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "samples",
		Short: "Recent telemetry samples",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := opt.openStore()
			if err != nil {
				return err
			}
			if db == nil {
				return exitcode.Usagef("history requires SQLite (omit --no-store)")
			}
			defer func() { _ = db.Close() }()
			rows, err := db.ListSamples(limit)
			if err != nil {
				return err
			}
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, rows)
			}
			table := make([][]string, 0, len(rows))
			for _, s := range rows {
				table = append(table, []string{
					fmt.Sprintf("%d", s.ID), s.RecordedAt, onOff(s.HeaterOn), fmtOpt(s.TempC), fmtOpt(s.TargetTempC), s.Profile,
				})
			}
			return writeHumanTable(cmd, opt, []string{"ID", "AT", "HEAT", "TEMP", "TARGET", "PROFILE"}, table, rows)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows")
	return cmd
}

func newHistorySessionsCmd(opt *Options) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Detected heat runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := opt.openStore()
			if err != nil {
				return err
			}
			if db == nil {
				return exitcode.Usagef("history requires SQLite (omit --no-store)")
			}
			defer func() { _ = db.Close() }()
			rows, err := db.ListSessions(limit)
			if err != nil {
				return err
			}
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, rows)
			}
			table := make([][]string, 0, len(rows))
			for _, s := range rows {
				ended := ""
				if s.EndedAt != nil {
					ended = *s.EndedAt
				}
				dur := ""
				if s.DurationS != nil {
					dur = fmt.Sprintf("%.0fs", *s.DurationS)
				}
				table = append(table, []string{
					fmt.Sprintf("%d", s.ID), s.StartedAt, ended, dur, fmtOpt(s.PeakTempC), s.Source,
				})
			}
			return writeHumanTable(cmd, opt, []string{"ID", "START", "END", "DUR", "PEAK_C", "SOURCE"}, table, rows)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max rows")
	return cmd
}

func newHistoryHoursCmd(opt *Options) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "hours",
		Short: "Hours heater was on (spend-style buckets)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := opt.openStore()
			if err != nil {
				return err
			}
			if db == nil {
				return exitcode.Usagef("history requires SQLite (omit --no-store)")
			}
			defer func() { _ = db.Close() }()
			rows, err := db.Hours(by)
			if err != nil {
				return err
			}
			if opt.JSON || opt.Agent {
				return writeOut(cmd, opt, rows)
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Bucket, fmt.Sprintf("%d", r.Sessions), fmt.Sprintf("%.2f", r.Hours)})
			}
			return writeHumanTable(cmd, opt, []string{"BUCKET", "SESSIONS", "HOURS"}, table, rows)
		},
	}
	cmd.Flags().StringVar(&by, "by", "week", "week or month")
	return cmd
}
