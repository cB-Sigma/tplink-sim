// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop runs python-kasa's client (peer/client.py, in a
// container) against the simulated devices: the legacy protocol and its
// UDP discovery, KLAP v1 with the Kasa JSON, KLAP v2 with Tapo requests,
// securePassthrough with both login versions, and the 20002 discovery,
// through python-kasa's protocol classes and through its Device API. The
// tests need Docker and are skipped unless SIGMA_TPLINK_INTEROP=1.
package interop

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink-sim/sim"
)

// run starts the client with args and returns its steps by name, once it
// prints "done".
func run(t *testing.T, args ...string) map[string]any {
	t.Helper()
	p := startPeer(t, append([]string{"--add-host=host.docker.internal:host-gateway", image, "python", "-u", "client.py"}, args...)...)
	steps := map[string]any{}
	deadline := time.After(120 * time.Second)
	for {
		select {
		case l := <-p.lines:
			name, js, _ := strings.Cut(l, " ")
			if name == "done" {
				return steps
			}
			var v any
			if js != "" {
				if err := json.Unmarshal([]byte(js), &v); err != nil {
					t.Fatalf("peer line %q: %v", l, err)
				}
			}
			steps[name] = v
			if strings.HasSuffix(name, "-error") {
				t.Errorf("python-kasa step %s: %v", name, v)
			}
		case <-deadline:
			t.Fatalf("python-kasa did not finish; steps %v", steps)
		}
	}
}

func port(t *testing.T, addr string) string {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func get(v any, path ...string) any {
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// The legacy protocol over TCP and its UDP discovery, against a simulated
// HS110 plug.
func TestKasaLegacy(t *testing.T) {
	needDocker(t)
	k, err := sim.StartKasa(map[string]any{"alias": "Kettle", "model": "HS110(EU)", "mac": "5C:62:8B:00:00:10", "mic_type": "IOT.SMARTPLUGSWITCH",
		"relay_state": 1.0, "deviceId": "8006A1", "hwId": "H1", "oemId": "O1", "sw_ver": "1.5.4 Build 180815", "hw_ver": "2.0", "type": "IOT.SMARTPLUGSWITCH",
		"feature": "TIM:ENE", "led_off": 0.0, "on_time": 10.0, "rssi": -50.0, "latitude_i": 0.0, "longitude_i": 0.0, "dev_name": "Smart Plug"},
		map[string]any{"voltage_mv": 229400.0, "current_ma": 910.0, "power_mw": 205300.0, "total_wh": 88213.0})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := run(t, "legacy", "host.docker.internal", port(t, k.Addr()))
	if get(s["query"], "system", "get_sysinfo", "alias") != "Kettle" {
		t.Errorf("query %v", s["query"])
	}
	if get(s["device"], "model") != "HS110" || get(s["device2"], "is_on") != true {
		t.Errorf("device %v %v", s["device"], s["device2"])
	}
	found, _ := s["discover-raw"].(map[string]any)
	if len(found) != 1 {
		t.Errorf("discovery %v", s["discover-raw"])
	}
	for _, d := range found {
		if get(d, "model") != "HS110" || get(d, "is_on") != true {
			t.Errorf("discovered %v", d)
		}
	}
}

// KLAP v1 with the Kasa JSON, against a simulated Kasa plug on newer
// firmware.
func TestKasaOnKLAPv1(t *testing.T) {
	needDocker(t)
	d, k := sim.StartKasaKLAP(sim.DemoUser, sim.DemoPassword, 1, map[string]any{"alias": "Plug", "model": "HS100(UK)", "mac": "5C:62:8B:00:00:A1",
		"mic_type": "IOT.SMARTPLUGSWITCH", "relay_state": 1.0, "deviceId": "8006A2", "hwId": "H1", "oemId": "O1", "sw_ver": "1.1.0", "hw_ver": "4.1",
		"type": "IOT.SMARTPLUGSWITCH", "feature": "TIM", "led_off": 0.0, "on_time": 10.0, "rssi": -50.0, "dev_name": "Smart Plug"}, nil)
	defer d.Close()
	s := run(t, "klap1", "host.docker.internal", port(t, d.Addr()), sim.DemoUser, sim.DemoPassword)
	if get(s["query"], "system", "get_sysinfo", "alias") != "Plug" {
		t.Errorf("query %v", s["query"])
	}
	if get(s["device2"], "is_on") != true || k.Get("relay_state") != 1.0 {
		t.Errorf("device %v, relay %v", s["device2"], k.Get("relay_state"))
	}
}

// KLAP v2 with Tapo requests, against a simulated P110.
func TestTapoOnKLAPv2(t *testing.T) {
	needDocker(t)
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, tapoInfo("P110", "3C-52-A1-00-00-04"), map[string]any{"current_power": 0.0, "today_energy": 120.0, "month_energy": 5400.0})
	defer p.Close()
	s := run(t, "klap2", "host.docker.internal", port(t, p.Addr()), sim.DemoUser, sim.DemoPassword)
	if get(s["query"], "get_device_info", "model") != "P110" || get(s["query2"], "get_device_info", "device_on") != true {
		t.Errorf("query %v %v", s["query"], s["query2"])
	}
	if get(s["device"], "model") != "P110" || get(s["device2"], "is_on") != false || p.Get("device_on") != false {
		t.Errorf("device %v %v, device_on %v", s["device"], s["device2"], p.Get("device_on"))
	}
}

// securePassthrough with login version 2 and 1, against a simulated P100.
func TestTapoOnSecurePassthrough(t *testing.T) {
	needDocker(t)
	for _, lv := range []string{"2", "1"} {
		t.Run("login"+lv, func(t *testing.T) {
			v := 2
			if lv == "1" {
				v = 1
			}
			p := sim.StartTapoAES(sim.DemoUser, sim.DemoPassword, v, tapoInfo("P100", "3C-52-A1-00-00-06"), nil)
			defer p.Close()
			s := run(t, "aes", "host.docker.internal", port(t, p.Addr()), sim.DemoUser, sim.DemoPassword, lv)
			if get(s["query"], "get_device_info", "model") != "P100" || get(s["query2"], "get_device_info", "device_on") != true {
				t.Errorf("query %v %v", s["query"], s["query2"])
			}
			if get(s["device2"], "is_on") != false || p.Get("device_on") != false {
				t.Errorf("device %v, device_on %v", s["device2"], p.Get("device_on"))
			}
		})
	}
}

// The 20002 discovery: python-kasa finds the simulated device, reads how
// to speak to it and reads it over that protocol.
func TestTapoDiscovery(t *testing.T) {
	needDocker(t)
	for _, aes := range []bool{false, true} {
		name := "klap"
		var d *sim.Tapo
		if aes {
			name = "aes"
			d = sim.StartTapoAES(sim.DemoUser, sim.DemoPassword, 2, tapoInfo("P100", "3C-52-A1-00-00-06"), nil)
		} else {
			d = sim.StartTapo(sim.DemoUser, sim.DemoPassword, tapoInfo("P110", "3C-52-A1-00-00-04"), nil)
		}
		t.Run(name, func(t *testing.T) {
			defer d.Close()
			r, err := sim.StartResponder(d)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			s := run(t, "tdp", "host.docker.internal", port(t, r.Addr()), sim.DemoUser, sim.DemoPassword)
			found, _ := s["discover"].(map[string]any)
			if len(found) != 1 {
				t.Fatalf("discovery %v", s["discover"])
			}
			for _, f := range found {
				want := "KLAP"
				if aes {
					want = "AES"
				}
				if get(f, "encrypt") != want || get(f, "type") != "SMART.TAPOPLUG" || get(f, "update_error") != nil {
					t.Errorf("discovered %v", f)
				}
			}
		})
	}
}

// tapoInfo is a device_info with the fields python-kasa's Device API
// reads.
func tapoInfo(model, mac string) map[string]any {
	return map[string]any{"device_id": "80223F" + strings.ReplaceAll(mac, "-", ""), "model": model, "type": "SMART.TAPOPLUG", "fw_ver": "1.3.0 Build 230905 Rel.152200",
		"hw_ver": "1.0", "mac": mac, "nickname": "UGx1Zw==", "device_on": true, "oem_id": "O1", "hw_id": "H1", "fw_id": "F1", "ip": "127.0.0.1",
		"ssid": "U2lnbWE=", "signal_level": 3.0, "rssi": -50.0, "lang": "en_US", "avatar": "plug", "region": "Europe/London", "specs": "",
		"time_diff": 0.0, "has_set_location_info": false, "on_time": 10.0, "overheated": false}
}
