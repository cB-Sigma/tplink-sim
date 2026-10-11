// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"strings"
	"testing"

	"github.com/cB-Sigma/tplink-sim/sim"
)

func tpapP110() map[string]any {
	return map[string]any{"device_id": "80223TPAP", "model": "P110", "type": "SMART.TAPOPLUG",
		"device_on": false, "fw_ver": "1.4.0 Build 250101 Rel.120000", "hw_ver": "1.0", "mac": "3C-52-A1-00-00-21", "nickname": "UGx1Zw==",
		"region": "Europe/London", "ip": "127.0.0.1", "specs": "", "lang": "en_US", "avatar": "plug", "time_diff": 0.0, "signal_level": 3.0,
		"rssi": -40.0, "on_time": 0.0, "overheated": false, "has_set_location_info": false, "oem_id": "O1", "hw_id": "H1", "latitude": 0.0, "longitude": 0.0,
		"model_type": "", "type_name": "", "default_states": map[string]any{"type": "last_states", "state": map[string]any{}},
		"overcurrent_status": "normal", "power_protection_status": "normal", "charging_status": "normal", "auto_off_status": "off",
		"auto_off_remain_time": 0.0}
}

// The tapo crate's client (Python bindings 0.11.1) logs in over TPAP to a
// simulated P110 in TPAP mode found by its address: it asks discover,
// runs SPAKE2+ and sends AES-CCM requests; it switches the plug and reads
// its energy use. A wrong password is refused as such.
func TestTapoTPAPPlug(t *testing.T) {
	needTapoImage(t)
	p := sim.StartTapoTPAP(sim.DemoUser, sim.DemoPassword, tpapP110(),
		map[string]any{"current_power": 0.0, "today_energy": 12.0, "month_energy": 300.0, "today_runtime": 5.0, "month_runtime": 50.0, "local_time": "2026-10-12 10:00:00"})
	defer p.Close()
	p.Do(func() { p.TPAP.Iterations = 3000 })
	s := runTapoScript(t, "tpap.py", nil, "plug", "host.docker.internal:"+port(t, p.Addr()), sim.DemoUser, sim.DemoPassword)
	if fe := s["fatal-error"]; fe != nil {
		t.Fatalf("tapo: %v", get(fe, "error"))
	}
	if get(s["info"], "model") != "P110" || get(s["info-on"], "device_on") != true || get(s["info-off"], "device_on") != false {
		t.Errorf("tapo read %v, %v, %v", s["info"], s["info-on"], s["info-off"])
	}
	if get(s["usage-dict"], "today_energy") != 12.0 {
		t.Errorf("usage %v", s["usage-dict"])
	}
	if n := p.TPAPLogins(); n != 1 {
		t.Errorf("%d TPAP logins", n)
	}
	t.Logf("tapo over TPAP: %v", s["info-on"])

	w := runTapoScript(t, "tpap.py", nil, "wrong", "host.docker.internal:"+port(t, p.Addr()), sim.DemoUser, sim.DemoPassword)
	if _, ok := w["wrong-accepted"]; ok || !strings.Contains(str(get(w["wrong"], "error")), "TPAP_CREDENTIALS") {
		t.Errorf("wrong password: %v", w)
	}
}

// The tapo crate's client logs in to a simulated C210 over TPAP on HTTPS
// (cameras by address: TPAP first, with the MD5 of the password).
func TestTapoTPAPCamera(t *testing.T) {
	needTapoImage(t)
	cam := sim.StartCameraTPAP("admin", "camera-pass", map[string]any{"device_type": "SMART.IPCAMERA", "device_model": "C220", "device_alias": "Porch",
		"sw_version": "1.3.11 Build 231117", "hw_version": "2.0", "mac": "00-31-92-00-00-22", "dev_id": "80223C22", "device_info": "C220 2.0 IPC",
		"device_name": "C220", "hw_id": "H1", "oem_id": "O1", "avatar": "camera", "has_set_location_info": 0, "longitude": 0, "latitude": 0,
		"region": "EU", "ffs": false, "dev_type": "SMART.IPCAMERA", "mobile_access": "0", "manufacturer_name": "TP-LINK", "sys_software_revision": "1",
		"sys_hardware_revision": "1", "features": 3, "status": "configured", "is_cal": true, "hw_desc": "x"})
	defer cam.Close()
	s := runTapoScript(t, "tpap.py", nil, "camera", "host.docker.internal:"+port(t, cam.Addr()), "admin", "camera-pass")
	if fe := s["fatal-error"]; fe != nil {
		t.Fatalf("tapo: %v", get(fe, "error"))
	}
	if s["info"] == nil {
		t.Fatalf("steps %v", s)
	}
	if n := cam.TPAPLogins(); n != 1 {
		t.Errorf("%d TPAP logins (the client fell back to the HTTPS login?)", n)
	}
	t.Logf("tapo over TPAP: %v", s["info"])
}

func str(v any) string { s, _ := v.(string); return s }
