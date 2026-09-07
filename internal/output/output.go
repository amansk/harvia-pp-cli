// Package output formats CLI results for humans and agents.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Mode selects encoding.
type Mode struct {
	JSON    bool
	Agent   bool
	Quiet   bool
	NoColor bool
}

// Compact is true when --agent is set.
func (m Mode) Compact() bool { return m.Agent }

// Encode writes a successful payload.
func (m Mode) Encode(w io.Writer, data any) error {
	return m.EncodeStatus(w, true, data, "")
}

// EncodeStatus writes a machine envelope whose top-level ok matches the outcome.
func (m Mode) EncodeStatus(w io.Writer, ok bool, data any, errMsg string) error {
	if m.Quiet && !m.JSON && !m.Agent {
		return nil
	}
	if m.JSON || m.Agent {
		env := map[string]any{"ok": ok, "data": data}
		if errMsg != "" {
			env["error"] = errMsg
		}
		return writeJSON(w, env, m.Compact())
	}
	return writeHuman(w, data)
}

// EncodeError writes a failed payload to w (typically stderr).
func (m Mode) EncodeError(w io.Writer, err error) error {
	if m.JSON || m.Agent {
		return writeJSON(w, map[string]any{"ok": false, "error": err.Error()}, m.Compact())
	}
	_, e := fmt.Fprintln(w, err.Error())
	return e
}

func writeJSON(w io.Writer, v any, compact bool) error {
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

func writeHuman(w io.Writer, data any) error {
	switch v := data.(type) {
	case string:
		_, err := fmt.Fprintln(w, v)
		return err
	case []string:
		for _, line := range v {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
		return nil
	default:
		b, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}
}

// Table writes aligned columns for human mode.
func Table(w io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	if err := writeRow(w, headers, widths); err != nil {
		return err
	}
	sep := make([]string, len(headers))
	for i, n := range widths {
		sep[i] = strings.Repeat("-", n)
	}
	if err := writeRow(w, sep, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(w, row, widths); err != nil {
			return err
		}
	}
	return nil
}

func writeRow(w io.Writer, cols []string, widths []int) error {
	parts := make([]string, len(widths))
	for i, n := range widths {
		cell := ""
		if i < len(cols) {
			cell = cols[i]
		}
		parts[i] = pad(cell, n)
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "  "))
	return err
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
