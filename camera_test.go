// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

func camSections() map[string]map[string]any {
	return map[string]map[string]any{
		"lens_mask":        {"lens_mask_info": map[string]any{"enabled": "off"}},
		"led":              {"config": map[string]any{"enabled": "on"}},
		"motion_detection": {"motion_det": map[string]any{"enabled": "on", "digital_sensitivity": "60", "sensitivity": "medium"}},
		"people_detection": {"detection": map[string]any{"enabled": "off", "sensitivity": "50"}},
		"sound_detection":  {"bcd": map[string]any{"enabled": "off", "sensitivity": "medium"}},
		"tamper_detection": {"tamper_det": map[string]any{"enabled": "on"}},
		"siren": {"status": "off", "siren_type": "Alarm 1", "volume": "5", "duration": 30.0,
			"siren_type_list": []any{"Alarm 1", "Alarm 2"}},
		"image": {"switch": map[string]any{"night_vision_mode": "inf_night_vision"}, "common": map[string]any{"inf_type": "auto"}},
		"msg_alarm": {"chn1_msg_alarm_info": map[string]any{"enabled": "off", "alarm_mode": []any{"sound", "light"}, "alarm_volume": "high",
			"alarm_duration": "0"}},
		"harddisk_manage": {"hd_info": []any{map[string]any{"hd_info_1": map[string]any{"disk_name": "1", "status": "normal",
			"total_space": "29.7GB", "free_space": "12.1GB"}}}},
	}
}

// Every typed camera helper against a simulated C210.
func TestTapoCameraHelpers(t *testing.T) {
	info := camInfo()
	info["battery_percent"], info["low_battery"], info["battery_charging"], info["power"] = 80.0, false, "NO", "BATTERY"
	cam := sim.StartCamera("admin", "camera-pass", info)
	defer cam.Close()
	day := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	cam.Do(func() {
		cam.CamSections = camSections()
		cam.Components = map[string]int{"ptz": 1, "lensMask": 2}
		cam.CamSections["system"] = map[string]any{"last_alarm_info": map[string]any{"last_alarm_time": "1750109746", "last_alarm_type": "motion"}}
		cam.Recordings = []sim.CamClip{{Start: day, End: day.Add(time.Minute), Type: 2}, {Start: day.Add(2 * time.Hour), End: day.Add(2*time.Hour + 30*time.Second), Type: 1},
			{Start: day.AddDate(0, 0, 1), End: day.AddDate(0, 0, 1).Add(time.Minute), Type: 6}}
	})
	c := &tplink.SSLAESClient{Base: "https://" + cam.Addr(), Username: "admin", Password: "camera-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	cm := tplink.TapoCamera{Caller: c}

	ci, err := cm.Info(ctx)
	if err != nil || ci.Model != "C210" || ci.Alias != "Porch" || ci.Battery == nil || *ci.Battery != 80 || ci.Charging || ci.Power != "BATTERY" {
		t.Fatalf("info %+v %v", ci, err)
	}
	if comp, err := cm.Components(ctx); err != nil || comp["ptz"] != 1 || comp["lensMask"] != 2 {
		t.Errorf("components %v %v", comp, err)
	}
	if err := cm.SetAlias(ctx, "Gate"); err != nil {
		t.Fatal(err)
	}
	if ci, _ := cm.Info(ctx); ci.Alias != "Gate" {
		t.Errorf("alias %q", ci.Alias)
	}

	// Pan and tilt, presets.
	if err := cm.Move(ctx, 10, -5); err != nil {
		t.Fatal(err)
	}
	if err := cm.MoveStep(ctx, 90); err != nil {
		t.Fatal(err)
	}
	if p, ti := cam.CamPosition(); p != 10 || ti != 5 {
		t.Errorf("position %d %d", p, ti)
	}
	if err := cm.MoveStep(ctx, 360); err == nil {
		t.Error("an angle of 360 was sent")
	}
	if err := cm.SavePreset(ctx, "Door"); err != nil {
		t.Fatal(err)
	}
	if err := cm.Calibrate(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ := cam.CamPosition(); p != 0 {
		t.Errorf("calibrated to %d", p)
	}
	ps, err := cm.Presets(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "Door" {
		t.Fatalf("presets %+v %v", ps, err)
	}
	if err := cm.GotoPreset(ctx, ps[0].ID); err != nil {
		t.Fatal(err)
	}
	if p, ti := cam.CamPosition(); p != 10 || ti != 5 {
		t.Errorf("at preset %d %d", p, ti)
	}
	if err := cm.DeletePreset(ctx, ps[0].ID); err != nil {
		t.Fatal(err)
	}
	if ps, _ := cm.Presets(ctx); len(ps) != 0 {
		t.Errorf("after delete %+v", ps)
	}

	// Privacy and the status light.
	if on, err := cm.Privacy(ctx); err != nil || on {
		t.Errorf("privacy %v %v", on, err)
	}
	if err := cm.SetPrivacy(ctx, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := cm.Privacy(ctx); !on {
		t.Error("privacy not on")
	}
	if err := cm.SetLED(ctx, false); err != nil {
		t.Fatal(err)
	}
	if on, err := cm.LED(ctx); err != nil || on {
		t.Errorf("LED %v %v", on, err)
	}

	// Detection settings.
	if d, err := cm.Detection(ctx, tplink.DetectMotion); err != nil || !d.Enabled || d.Sensitivity != "60" {
		t.Errorf("motion %+v %v", d, err)
	}
	if err := cm.SetDetection(ctx, tplink.DetectPerson, true); err != nil {
		t.Fatal(err)
	}
	if d, err := cm.Detection(ctx, tplink.DetectPerson); err != nil || !d.Enabled || d.Sensitivity != "50" {
		t.Errorf("person %+v %v", d, err)
	}
	if err := cm.SetDetectionConfig(ctx, tplink.DetectBabyCry, map[string]any{"enabled": "on", "sensitivity": "high"}); err != nil {
		t.Fatal(err)
	}
	if d, _ := cm.Detection(ctx, tplink.DetectBabyCry); !d.Enabled || d.Sensitivity != "high" {
		t.Errorf("baby cry %+v", d)
	}
	if d, _ := cm.Detection(ctx, tplink.DetectTamper); !d.Enabled {
		t.Errorf("tamper %+v", d)
	}
	var se *tplink.SmartError
	if _, err := cm.Detection(ctx, tplink.DetectPet); !errors.As(err, &se) {
		t.Errorf("a detection the camera lacks: %v", err)
	}
	if _, err := cm.Detection(ctx, "smell"); err == nil {
		t.Error("an unknown detection was sent")
	}
	if la, err := cm.LastAlarm(ctx); err != nil || la.Type != "motion" || la.Time.Unix() != 1750109746 {
		t.Errorf("last alarm %+v %v", la, err)
	}

	// Siren, alert, manual alarm.
	s, err := cm.Siren(ctx)
	if err != nil || s.Active || s.Sound != "Alarm 1" || s.Volume != 5 || s.Duration != 30 || !reflect.DeepEqual(s.Sounds, []string{"Alarm 1", "Alarm 2"}) {
		t.Errorf("siren %+v %v", s, err)
	}
	if err := cm.SetSiren(ctx, "Alarm 2", 8, 0); err != nil {
		t.Fatal(err)
	}
	if err := cm.SoundSiren(ctx, true); err != nil {
		t.Fatal(err)
	}
	if s, _ := cm.Siren(ctx); !s.Active || s.Sound != "Alarm 2" || s.Volume != 8 || s.Duration != 30 {
		t.Errorf("siren after %+v", s)
	}
	if a, err := cm.Alert(ctx); err != nil || a.Enabled || len(a.Modes) != 2 || a.Volume != "high" {
		t.Errorf("alert %+v %v", a, err)
	}
	if err := cm.SetAlert(ctx, map[string]any{"enabled": "on"}); err != nil {
		t.Fatal(err)
	}
	if a, _ := cm.Alert(ctx); !a.Enabled {
		t.Error("alert not on")
	}
	if err := cm.ManualAlarm(ctx, true); err != nil || !cam.CamAlarm() {
		t.Errorf("manual alarm %v", err)
	}
	if err := cm.ManualAlarm(ctx, false); err != nil || cam.CamAlarm() {
		t.Errorf("manual alarm stop %v", err)
	}

	// Image.
	if m, err := cm.NightVision(ctx); err != nil || m != tplink.NightVisionInfrared {
		t.Errorf("night vision %q %v", m, err)
	}
	if err := cm.SetNightVision(ctx, tplink.NightVisionSmart); err != nil {
		t.Fatal(err)
	}
	if m, _ := cm.NightVision(ctx); m != tplink.NightVisionSmart {
		t.Errorf("night vision after %q", m)
	}
	if err := cm.SetDayNight(ctx, "on"); err != nil {
		t.Fatal(err)
	}
	if m, err := cm.DayNight(ctx); err != nil || m != "on" {
		t.Errorf("day night %q %v", m, err)
	}

	// Clock.
	ct, err := cm.Time(ctx)
	if err != nil || ct.ZoneID != "Europe/London" || time.Since(ct.Time) > time.Minute {
		t.Errorf("time %+v %v", ct, err)
	}
	if err := cm.SetTimezone(ctx, "Asia/Kolkata", tplink.TimezoneOffset(330*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if ct, _ := cm.Time(ctx); ct.ZoneID != "Asia/Kolkata" || ct.Timezone != "UTC+05:30" {
		t.Errorf("time zone %+v", ct)
	}
	if got := tplink.TimezoneOffset(-90 * time.Minute); got != "UTC-01:30" {
		t.Errorf("offset %q", got)
	}

	// Storage and recordings.
	disks, err := cm.SDCards(ctx)
	if err != nil || len(disks) != 1 || disks[0].Status != "normal" || disks[0].TotalSpace != "29.7GB" {
		t.Errorf("disks %+v %v", disks, err)
	}
	if err := cm.FormatSDCard(ctx); err != nil {
		t.Fatal(err)
	}
	days, err := cm.RecordingDays(ctx, day.AddDate(0, 0, -3), day.AddDate(0, 0, 3), "")
	if err != nil || !reflect.DeepEqual(days, []string{"20261010", "20261011"}) {
		t.Errorf("days %v %v", days, err)
	}
	recs, err := cm.Recordings(ctx, day, day.Add(24*time.Hour-time.Second), 0, 0, "")
	if err != nil || len(recs) != 2 || recs[0].Type != 2 || !recs[0].Start.Equal(day) || !recs[1].End.Equal(day.Add(2*time.Hour+30*time.Second)) {
		t.Errorf("recordings %+v %v", recs, err)
	}

	if nets, err := cm.WifiScan(ctx); err != nil || len(nets) != 1 || nets[0].SSID != "Demo-Home" {
		t.Errorf("scan %+v %v", nets, err)
	}
	if conn, err := cm.Connection(ctx); err != nil || conn.Link != "wifi" || conn.RSSI != -50 || conn.SSID != "Demo-Home" {
		t.Errorf("connection %+v %v", conn, err)
	}
	if err := cm.Reboot(ctx); err != nil {
		t.Fatal(err)
	}
	if reb, formats, cal, single := cam.CamCounts(); reb != 1 || formats != 1 || cal != 1 || single != 2 {
		t.Errorf("counts %d %d %d %d", reb, formats, cal, single)
	}
	if cam.TagFailures() != 0 {
		t.Errorf("%d tag failures", cam.TagFailures())
	}
}

// A camera that refuses motorMove is turned with the single "do" form; a
// camera behind a hub cannot take the single form.
func TestTapoCameraSingleForm(t *testing.T) {
	cam := sim.StartCamera("admin", "camera-pass", camInfo())
	defer cam.Close()
	c := &tplink.SSLAESClient{Base: "https://" + cam.Addr(), Username: "admin", Password: "camera-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if _, err := tplink.CamSingle(ctx, c, "do", "motor", map[string]any{"move": map[string]any{"x_coord": "3", "y_coord": "4"}}); err != nil {
		t.Fatal(err)
	}
	if p, ti := cam.CamPosition(); p != 3 || ti != 4 {
		t.Errorf("position %d %d", p, ti)
	}
	if err := (tplink.TapoCamera{Caller: c, ChildID: "child"}).ManualAlarm(ctx, true); !errors.Is(err, tplink.ErrNoSingleRequests) {
		t.Errorf("child single form: %v", err)
	}
	if _, err := tplink.CamSingle(ctx, tplink.TapoDevice{Caller: c}, "do", "motor", nil); !errors.Is(err, tplink.ErrNoSingleRequests) {
		t.Errorf("no Send: %v", err)
	}
}
