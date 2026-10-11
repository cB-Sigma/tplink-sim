// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"strings"
	"testing"

	"github.com/cB-Sigma/tplink-sim/sim"
)

// python-kasa's camera modules through its Device API against a simulated
// C210 with pan and tilt, privacy, the status light, four detections and a
// siren: every feature read, settings changed, the motor turned, a preset
// saved and the siren sounded, each checked in the simulator.
func TestCameraModules(t *testing.T) {
	needDocker(t)
	cam := sim.StartCamera("admin", "camera-pass", camInfo("SMART.IPCAMERA", "C210"))
	defer cam.Close()
	cam.Do(func() {
		cam.Components = map[string]int{"led": 1, "lensMask": 2, "ptz": 1, "detection": 3, "personDetection": 2, "babyCryDetection": 1,
			"tamperDetection": 1, "siren": 2}
		cam.CamSections = map[string]map[string]any{
			"lens_mask":        {"lens_mask_info": map[string]any{"enabled": "off"}},
			"led":              {"config": map[string]any{"enabled": "on"}},
			"motion_detection": {"motion_det": map[string]any{"enabled": "on", "digital_sensitivity": "60", "sensitivity": "medium"}},
			"people_detection": {"detection": map[string]any{"enabled": "off", "sensitivity": "50"}},
			"sound_detection":  {"bcd": map[string]any{"enabled": "off", "sensitivity": "medium"}},
			"tamper_detection": {"tamper_det": map[string]any{"enabled": "on", "digital_sensitivity": "50"}},
			"siren":            {"status": "off", "siren_type": "Alarm 1", "volume": "5", "duration": 30.0, "siren_type_list": []any{"Alarm 1", "Alarm 2"}},
			"preset":           {"preset": map[string]any{"id": []any{}, "name": []any{}, "position_pan": []any{}, "position_tilt": []any{}, "read_only": []any{}}},
			"system":           {"last_alarm_info": map[string]any{"last_alarm_time": "1750109746", "last_alarm_type": "motion"}},
		}
	})
	s := run(t, "camera-modules", "host.docker.internal", port(t, cam.Addr()), "admin", "camera-pass")
	for k, v := range s {
		if strings.HasSuffix(k, "-error") {
			t.Errorf("%s: %v", k, get(v, "error"))
		}
	}
	mods, _ := s["modules"].([]any)
	have := map[string]bool{}
	for _, m := range mods {
		have[m.(string)] = true
	}
	for _, m := range []string{"Led", "LensMask", "PanTilt", "MotionDetection", "PersonDetection", "BabyCryDetection", "TamperDetection", "Alarm"} {
		if !have[m] {
			t.Errorf("python-kasa did not load %s (modules %v)", m, mods)
		}
	}
	f := s["features"]
	if get(f, "led") != true || get(f, "motion_detection") != true || get(f, "person_detection") != false || get(f, "tamper_detection") != true ||
		get(f, "alarm_sound") != "Alarm 1" || get(f, "alarm_volume") != 5.0 {
		t.Errorf("features %v", f)
	}
	f2 := s["features2"]
	if get(f2, "led") != false || get(f2, "motion_detection") != false || get(f2, "person_detection") != true || get(f2, "baby_cry_detection") != true ||
		get(f2, "alarm") != true || get(f2, "alarm_volume") != 3.0 {
		t.Errorf("features after %v", f2)
	}
	var lens, led any
	cam.Do(func() {
		lens = cam.CamSections["lens_mask"]["lens_mask_info"].(map[string]any)["enabled"]
		led = cam.CamSections["led"]["config"].(map[string]any)["enabled"]
	})
	if lens != "on" || led != "off" {
		t.Errorf("simulator lens mask %v, LED %v", lens, led)
	}
	if p, ti := cam.CamPosition(); p != 10 || ti != -5 {
		t.Errorf("simulator position %d %d", p, ti)
	}
	if cam.TagFailures() != 0 {
		t.Errorf("%d tag failures", cam.TagFailures())
	}
}
