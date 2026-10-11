// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"testing"

	"github.com/cB-Sigma/tplink-sim/sim"
)

// python-kasa's KLAP over HTTPS against the simulated device.
func TestKLAPOverHTTPS(t *testing.T) {
	needDocker(t)
	p := sim.StartTapoHTTPS(sim.DemoUser, sim.DemoPassword, tapoInfo("P110M", "3C-52-A1-00-00-09"), nil)
	defer p.Close()
	s := run(t, "klap-https", "host.docker.internal", port(t, p.Addr()), sim.DemoUser, sim.DemoPassword)
	if get(s["query"], "get_device_info", "model") != "P110M" || get(s["device2"], "is_on") != false || p.Get("device_on") != false {
		t.Errorf("query %v device %v", s["query"], s["device2"])
	}
}

func camInfo(typ, model string) map[string]any {
	return map[string]any{"device_type": typ, "device_model": model, "device_alias": "Porch", "device_name": "Porch", "sw_version": "1.3.11 Build 231117 Rel.39429n",
		"hw_version": "2.0", "mac": "00-31-92-00-00-0A", "dev_id": "80223CAM000000000000000000000000000000", "hw_id": "H1", "oem_id": "O1",
		"device_info": model + " 2.0 IPC", "region": "EU", "avatar": "Home", "features": 3, "ffs": false, "has_set_location_info": 1}
}

// python-kasa's camera Device API on the HTTPS transport against a
// simulated C210: connect (login, signed requests), information, rename.
func TestCamera(t *testing.T) {
	needDocker(t)
	cam := sim.StartCamera("admin", "camera-pass", camInfo("SMART.IPCAMERA", "C210"))
	defer cam.Close()
	s := run(t, "camera", "host.docker.internal", port(t, cam.Addr()), "admin", "camera-pass")
	if get(s["device"], "model") != "C210" || get(s["device"], "type") != "DeviceType.Camera" || get(s["device"], "fw") != "1.3.11 Build 231117 Rel.39429n" {
		t.Errorf("device %v", s["device"])
	}
	if cam.Get("device_alias") != "Renamed by python-kasa" || get(s["device2"], "alias") != "Renamed by python-kasa" {
		t.Errorf("alias %v, python-kasa %v", cam.Get("device_alias"), s["device2"])
	}
	if cam.TagFailures() != 0 {
		t.Errorf("python-kasa's requests failed the tag check %d times", cam.TagFailures())
	}
}

// python-kasa's H200 hub on the HTTPS transport with a sensor child.
func TestH200Hub(t *testing.T) {
	needDocker(t)
	h := sim.StartCamera("admin", "hub-pass", camInfo("SMART.TAPOHUB", "H200"))
	defer h.Close()
	h.Do(func() {
		h.Components = map[string]int{"childControl": 1}
		h.Children = []map[string]any{{"device_id": "T1", "model": "T315", "category": "subg.trigger.temp-hmdt-sensor", "type": "SMART.TAPOSENSOR",
			"current_temp": 21.5, "current_humidity": 52.0, "temp_unit": "celsius", "current_temp_exception": 0.0, "current_humidity_exception": 0.0,
			"nickname": "U2Vuc29y", "status": "online", "fw_ver": "1.7.0 Build 230424", "hw_ver": "1.0", "mac": "F0A731000000", "at_low_battery": false,
			"battery_percentage": 90.0, "rssi": -45.0, "signal_level": 3.0, "region": "Europe/London", "specs": "EU", "oem_id": "O1", "hw_id": "H1",
			"parent_device_id": "80223CAM", "lastOnboardingTimestamp": 1706990901.0, "jamming_rssi": -118.0, "avatar": "", "bind_count": 1.0,
			"status_follow_edge": false, "report_interval": 16.0}}
	})
	s := run(t, "camera-hub", "host.docker.internal", port(t, h.Addr()), "admin", "hub-pass")
	if get(s["device"], "type") != "DeviceType.Hub" {
		t.Errorf("device %v", s["device"])
	}
	if kids, _ := get(s["device"], "children").([]any); len(kids) != 1 || kids[0] != "T1" {
		t.Errorf("children %v", s["device"])
	}
	if get(s["child"], "model") != "T315" || get(s["child"], "features", "temperature") != 21.5 {
		t.Errorf("child %v", s["child"])
	}
}

// python-kasa's Kasa camera transport against the simulated camera.
func TestKasaCameraLinkie(t *testing.T) {
	needDocker(t)
	k := sim.StartKasaCamera("unused", "unused", map[string]any{"alias": "Garage", "model": "KC100(US)", "type": "IOT.IPCAMERA", "mac": "74:FE:CE:00:00:01"})
	defer k.Close()
	k.Do(func() { k.AnyLogin = true }) // python-kasa sends its built-in default login
	s := run(t, "linkie", "host.docker.internal", port(t, k.Addr()))
	if get(s["query"], "system", "get_sysinfo", "model") != "KC100(US)" || k.Get("alias") != "Renamed by python-kasa" {
		t.Errorf("query %v alias %v", s["query"], k.Get("alias"))
	}
}

// python-kasa's plain HTTPS transport (robot vacuums) against the
// simulated device.
func TestTapoPlainHTTPS(t *testing.T) {
	needDocker(t)
	v := sim.StartTapoSSL(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "RV30 Max", "type": "SMART.TAPOROBOVAC", "device_id": "80223V"})
	defer v.Close()
	s := run(t, "plain-https", "host.docker.internal", port(t, v.Addr()), sim.DemoUser, sim.DemoPassword)
	if s["transport"] != "SslTransport" || get(s["query"], "get_device_info", "model") != "RV30 Max" || get(s["multi"], "get_device_time", "timestamp") == nil {
		t.Errorf("transport %v query %v multi %v", s["transport"], s["query"], s["multi"])
	}
}
