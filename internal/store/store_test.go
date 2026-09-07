package store

import (
	"testing"
	"time"

	"github.com/amansk/harvia-pp-cli/internal/actions"
)

func TestRecordSampleOpensAndClosesSession(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	temp := 40.0
	t0 := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	off := actions.Status{DeviceID: "dev", HeaterOn: false, TempC: &temp, Profile: "Cozy"}
	on := actions.Status{DeviceID: "dev", HeaterOn: true, TempC: &temp, Profile: "Custom"}
	peak := 70.0
	onHot := actions.Status{DeviceID: "dev", HeaterOn: true, TempC: &peak, Profile: "Custom"}

	if err := db.RecordSample(off, nil, nil, "status", t0); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSample(on, nil, nil, "cli", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSample(onHot, nil, nil, "watch", t0.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSample(off, nil, nil, "status", t0.Add(40*time.Minute)); err != nil {
		t.Fatal(err)
	}

	sessions, err := db.ListSessions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions %d", len(sessions))
	}
	s := sessions[0]
	if s.EndedAt == nil || s.PeakTempC == nil || *s.PeakTempC != 70 {
		t.Fatalf("%+v", s)
	}
	if s.Source != "cli" {
		t.Fatalf("source %s", s.Source)
	}

	n, err := db.SampleCount()
	if err != nil || n != 4 {
		t.Fatalf("samples %d %v", n, err)
	}

	hours, err := db.Hours("month")
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 1 || hours[0].Sessions != 1 {
		t.Fatalf("%+v", hours)
	}
	if hours[0].Hours < 0.6 || hours[0].Hours > 0.7 {
		t.Fatalf("hours %f", hours[0].Hours)
	}
}
