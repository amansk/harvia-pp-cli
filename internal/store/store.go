// Package store is the optional local SQLite telemetry cache.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/actions"
	"github.com/amansk/harvia-pp-cli/internal/auth"
	"github.com/amansk/harvia-pp-cli/internal/exitcode"
	_ "modernc.org/sqlite"
)

// DB wraps sqlite.
type DB struct {
	sql  *sql.DB
	Path string
}

// Open creates/migrates $home/harvia.db.
func Open(home string) (*DB, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("store home: %w", err)
	}
	path := filepath.Join(home, auth.DBFile)
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("pragma: %w", err)
	}
	d := &DB{sql: sqlDB, Path: path}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recorded_at TEXT NOT NULL,
  device_id TEXT NOT NULL,
  heater_on INTEGER,
  temp_c REAL,
  target_temp_c REAL,
  light_on INTEGER,
  fan_on INTEGER,
  door_closed INTEGER,
  auto_off_min REAL,
  profile TEXT,
  raw_state_json TEXT,
  raw_telemetry_json TEXT
);
CREATE TABLE IF NOT EXISTS sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id TEXT NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  peak_temp_c REAL,
  source TEXT NOT NULL DEFAULT 'external'
);
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_samples_recorded ON samples(recorded_at);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
`
	if _, err := d.sql.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	_, err := d.sql.Exec(`PRAGMA user_version = 1`)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func optFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// RecordSample appends a status snapshot and updates the open session.
func (d *DB) RecordSample(st actions.Status, rawState, rawTele []byte, source string, at time.Time) error {
	if source == "" {
		source = "external"
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	stamp := at.UTC().Format(time.RFC3339)
	if rawState == nil {
		rawState = []byte("{}")
	}
	if rawTele == nil {
		rawTele = []byte("{}")
	}
	_, err := d.sql.Exec(`
INSERT INTO samples (recorded_at, device_id, heater_on, temp_c, target_temp_c, light_on, fan_on, door_closed, auto_off_min, profile, raw_state_json, raw_telemetry_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		stamp, st.DeviceID, boolInt(st.HeaterOn), optFloat(st.TempC), optFloat(st.TargetTempC),
		boolInt(st.LightOn), boolInt(st.FanOn), boolInt(st.DoorClosed), optFloat(st.AutoOffMin),
		st.Profile, string(rawState), string(rawTele),
	)
	if err != nil {
		return err
	}
	return d.touchSession(st, source, stamp)
}

func (d *DB) openSessionID(deviceID string) (int64, bool, error) {
	var id int64
	err := d.sql.QueryRow(`SELECT id FROM sessions WHERE device_id = ? AND ended_at IS NULL ORDER BY id DESC LIMIT 1`, deviceID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return id, err == nil, err
}

func (d *DB) touchSession(st actions.Status, source, stamp string) error {
	id, open, err := d.openSessionID(st.DeviceID)
	if err != nil {
		return err
	}
	if st.HeaterOn {
		if !open {
			_, err := d.sql.Exec(`INSERT INTO sessions (device_id, started_at, peak_temp_c, source) VALUES (?, ?, ?, ?)`,
				st.DeviceID, stamp, optFloat(st.TempC), source)
			return err
		}
		if st.TempC != nil {
			_, err := d.sql.Exec(`UPDATE sessions SET peak_temp_c = CASE
				WHEN peak_temp_c IS NULL OR peak_temp_c < ? THEN ? ELSE peak_temp_c END
				WHERE id = ?`, *st.TempC, *st.TempC, id)
			return err
		}
		return nil
	}
	if open {
		_, err := d.sql.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, stamp, id)
		return err
	}
	return nil
}

// Sample is one stored snapshot.
type Sample struct {
	ID          int64    `json:"id"`
	RecordedAt  string   `json:"recorded_at"`
	DeviceID    string   `json:"device_id"`
	HeaterOn    bool     `json:"heater_on"`
	TempC       *float64 `json:"temp_c"`
	TargetTempC *float64 `json:"target_temp_c"`
	LightOn     bool     `json:"light_on"`
	FanOn       bool     `json:"fan_on"`
	DoorClosed  bool     `json:"door_closed"`
	AutoOffMin  *float64 `json:"auto_off_min"`
	Profile     string   `json:"profile"`
}

// Session is one heat run.
type Session struct {
	ID        int64    `json:"id"`
	DeviceID  string   `json:"device_id"`
	StartedAt string   `json:"started_at"`
	EndedAt   *string  `json:"ended_at"`
	PeakTempC *float64 `json:"peak_temp_c"`
	Source    string   `json:"source"`
	DurationS *float64 `json:"duration_s,omitempty"`
}

func scanOptFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

// ListSamples returns newest first.
func (d *DB) ListSamples(limit int) ([]Sample, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.sql.Query(`
SELECT id, recorded_at, device_id, heater_on, temp_c, target_temp_c, light_on, fan_on, door_closed, auto_off_min, profile
FROM samples ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sample
	for rows.Next() {
		var s Sample
		var heater, light, fan, door int
		var temp, target, auto sql.NullFloat64
		if err := rows.Scan(&s.ID, &s.RecordedAt, &s.DeviceID, &heater, &temp, &target, &light, &fan, &door, &auto, &s.Profile); err != nil {
			return nil, err
		}
		s.HeaterOn = heater != 0
		s.LightOn = light != 0
		s.FanOn = fan != 0
		s.DoorClosed = door != 0
		s.TempC = scanOptFloat(temp)
		s.TargetTempC = scanOptFloat(target)
		s.AutoOffMin = scanOptFloat(auto)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListSessions returns newest first.
func (d *DB) ListSessions(limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.sql.Query(`
SELECT id, device_id, started_at, ended_at, peak_temp_c, source
FROM sessions ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		var ended sql.NullString
		var peak sql.NullFloat64
		if err := rows.Scan(&s.ID, &s.DeviceID, &s.StartedAt, &ended, &peak, &s.Source); err != nil {
			return nil, err
		}
		if ended.Valid {
			s.EndedAt = &ended.String
			if dur := durationSeconds(s.StartedAt, ended.String); dur != nil {
				s.DurationS = dur
			}
		}
		s.PeakTempC = scanOptFloat(peak)
		out = append(out, s)
	}
	return out, rows.Err()
}

func durationSeconds(start, end string) *float64 {
	a, err1 := time.Parse(time.RFC3339, start)
	b, err2 := time.Parse(time.RFC3339, end)
	if err1 != nil || err2 != nil {
		return nil
	}
	s := b.Sub(a).Seconds()
	if s < 0 {
		s = 0
	}
	return &s
}

// HoursRow is a spend-style bucket of heater-on time.
type HoursRow struct {
	Bucket   string  `json:"bucket"`
	Sessions int     `json:"sessions"`
	Hours    float64 `json:"hours"`
}

// Hours groups closed sessions by week or month.
func (d *DB) Hours(by string) ([]HoursRow, error) {
	var expr string
	switch by {
	case "month":
		expr = `substr(started_at, 1, 7)`
	case "week":
		expr = `strftime('%Y-W%W', started_at)`
	default:
		return nil, exitcode.Usagef("--by must be week or month")
	}
	rows, err := d.sql.Query(fmt.Sprintf(`
SELECT COALESCE(%s, 'unknown'), COUNT(*),
  SUM(CASE WHEN ended_at IS NOT NULL
    THEN (julianday(ended_at) - julianday(started_at)) * 24.0
    ELSE 0 END)
FROM sessions
GROUP BY 1
ORDER BY 1 DESC`, expr))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HoursRow
	for rows.Next() {
		var r HoursRow
		var hours sql.NullFloat64
		if err := rows.Scan(&r.Bucket, &r.Sessions, &hours); err != nil {
			return nil, err
		}
		if hours.Valid {
			r.Hours = hours.Float64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SampleCount is used by doctor.
func (d *DB) SampleCount() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n)
	return n, err
}

// MustJSON is a small helper for tests/debug.
func MustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
