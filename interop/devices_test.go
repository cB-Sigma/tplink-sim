// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

func kasaPlugInfo() map[string]any {
	return map[string]any{"alias": "Kettle", "model": "HS110(EU)", "mac": "5C:62:8B:00:00:10", "mic_type": "IOT.SMARTPLUGSWITCH",
		"relay_state": 1.0, "deviceId": "8006A1", "hwId": "H1", "oemId": "O1", "sw_ver": "1.5.4 Build 180815", "hw_ver": "2.0", "type": "IOT.SMARTPLUGSWITCH",
		"feature": "TIM:ENE", "led_off": 0.0, "on_time": 10.0, "rssi": -50.0, "latitude_i": 0.0, "longitude_i": 0.0, "dev_name": "Smart Plug"}
}

// python-kasa's legacy Device API against a simulated plug: energy and
// runtime statistics, rule lists, a rule deleted, LED, name, erase, Wi-Fi
// scan and join, reboot.
func TestKasaPlugModules(t *testing.T) {
	needDocker(t)
	k, err := sim.StartKasa(kasaPlugInfo(), map[string]any{"voltage_mv": 229400.0, "current_ma": 910.0, "power_mw": 205300.0, "total_wh": 88213.0})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	// A schedule rule and a countdown on the device before python-kasa reads them.
	k.Do(func() { k.Networks = []map[string]any{{"ssid": "Demo-Home", "key_type": 3.0, "rssi": -40.0}} })
	if _, err := addKasaRule(k); err != nil {
		t.Fatal(err)
	}
	s := run(t, "kasa-plug", "host.docker.internal", port(t, k.Addr()))
	now := time.Now()
	if get(s["energy"], "power") != 205.3 {
		t.Errorf("energy %v", s["energy"])
	}
	daily, _ := s["daily"].(map[string]any)
	if len(daily) != now.Day() || daily["1"] != sim.DemoDayWh(int(now.Month()), 1) {
		t.Errorf("daily %v", s["daily"])
	}
	if m, _ := s["monthly"].(map[string]any); len(m) != int(now.Month()) {
		t.Errorf("monthly %v", s["monthly"])
	}
	if u, _ := s["usage-daily"].(map[string]any); len(u) != now.Day() {
		t.Errorf("usage %v", s["usage-daily"])
	}
	if r, _ := s["schedule"].([]any); len(r) != 1 || get(r[0], "name") != "evening" {
		t.Errorf("schedule %v", s["schedule"])
	}
	if len(k.Rules("schedule")) != 0 {
		t.Errorf("python-kasa's delete_rule left %v", k.Rules("schedule"))
	}
	if k.Get("led_off") != 1.0 || k.Get("alias") != "Renamed by python-kasa" || k.Reboots() != 1 {
		t.Errorf("led %v alias %v reboots %d", k.Get("led_off"), k.Get("alias"), k.Reboots())
	}
	if sc, _ := s["scan"].([]any); len(sc) != 1 || get(sc[0], "ssid") != "Demo-Home" {
		t.Errorf("scan %v", s["scan"])
	}
	if j := k.Joined(); j["ssid"] != "Demo-Home" || j["password"] != "wifi-password" || j["key_type"] != 3.0 {
		t.Errorf("joined %v", j)
	}
}

// addKasaRule adds a rule as an app would, on the simulator's own port.
func addKasaRule(k *sim.Kasa) (map[string]any, error) {
	return kasaCall(k.Addr(), map[string]any{"schedule": map[string]any{"add_rule": map[string]any{"name": "evening", "enable": 1, "wday": []int{1, 1, 1, 1, 1, 1, 1},
		"repeat": 1, "sact": 1, "stime_opt": 2, "smin": 0, "eact": -1, "etime_opt": -1, "emin": 0, "year": 0, "month": 0, "day": 0, "force": 0,
		"latitude": 0, "longitude": 0}}})
}

func kasaCall(addr string, req map[string]any) (map[string]any, error) {
	return tplink.KasaRequest(context.Background(), addr, req, 3*time.Second)
}

// python-kasa's dimmer: a transition and a fade time.
func TestKasaDimmer(t *testing.T) {
	needDocker(t)
	info := kasaPlugInfo()
	info["model"], info["dev_name"], info["brightness"], info["relay_state"], info["feature"] = "HS220(US)", "Smart Wi-Fi Dimmer", 20.0, 0.0, "TIM"
	k, err := sim.StartKasa(info, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := run(t, "kasa-dimmer", "host.docker.internal", port(t, k.Addr()))
	if get(s["device"], "type") != "DeviceType.Dimmer" || get(s["device2"], "brightness") != 70.0 || get(s["device2"], "is_on") != true {
		t.Errorf("dimmer %v %v", s["device"], s["device2"])
	}
	if k.DimmerSetting("fadeOnTime") != 2500.0 {
		t.Errorf("fade on time %v", k.DimmerSetting("fadeOnTime"))
	}
}

// python-kasa's bulb and light strip: colour with a transition, and a
// built-in effect on the strip.
func TestKasaLights(t *testing.T) {
	needDocker(t)
	for _, strip := range []bool{false, true} {
		name := "bulb"
		info := map[string]any{"alias": "Lamp", "model": "KL130(EU)", "mic_mac": "5C628B000003", "mic_type": "IOT.SMARTBULB", "sw_ver": "1.8.11", "hw_ver": "1.0",
			"deviceId": "8006B1", "hwId": "H1", "oemId": "O1", "rssi": -50.0, "is_color": 1.0, "is_dimmable": 1.0, "is_variable_color_temp": 1.0,
			"dev_state": "normal", "description": "Smart Wi-Fi LED Bulb with Color Changing", "is_factory": false, "disco_ver": "1.0", "ctrl_protocols": map[string]any{"name": "Linkie", "version": "1.0"},
			"active_mode": "none", "heapsize": 300000.0, "preferred_state": []any{}, "err_code": 0.0,
			"light_state": map[string]any{"on_off": 1.0, "mode": "normal", "brightness": 70.0, "hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}}
		if strip {
			name = "strip"
			info["model"], info["length"], info["description"] = "KL430(US)", 16.0, "Kasa Smart Light Strip, Multicolor"
			info["lighting_effect_state"] = map[string]any{"enable": 0.0, "name": "", "brightness": 50.0, "custom": 0.0, "id": ""}
		}
		t.Run(name, func(t *testing.T) {
			k, err := sim.StartKasa(info, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			s := run(t, "kasa-light", "host.docker.internal", port(t, k.Addr()))
			if hsv, _ := get(s["device2"], "hsv").([]any); len(hsv) != 3 || hsv[0] != 240.0 || (!strip && hsv[2] != 60.0) { // an effect sets a strip's brightness
				t.Errorf("hsv %v", s["device2"])
			}
			if ls := k.Get("light_state").(map[string]any); ls["hue"] != 240.0 {
				t.Errorf("light state %v", ls)
			}
			if strip {
				st := k.Get("lighting_effect_state").(map[string]any)
				if st["enable"] != 1.0 || st["name"] == "" || get(s["device2"], "effect") != st["name"] {
					t.Errorf("effect %v, python-kasa %v", st, s["device2"])
				}
			}
		})
	}
}

// python-kasa's Tapo plug: energy (component version 2), a firmware
// update run to its end, auto-off, LED, Wi-Fi scan and reboot.
func TestTapoPlugModules(t *testing.T) {
	needDocker(t)
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, tapoInfo("P110", "3C-52-A1-00-00-04"), map[string]any{"current_power": 12500.0, "today_energy": 120.0,
		"month_energy": 5400.0, "today_runtime": 60.0, "month_runtime": 900.0, "local_time": "2026-10-11 10:00:00"})
	defer p.Close()
	const latest = "1.4.0 Build 260901 Rel.100000"
	p.Do(func() {
		p.Components = map[string]int{"firmware": 2, "auto_off": 2, "led": 1, "wireless": 1, "cloud_connect": 1}
		p.Info["auto_off_status"], p.Info["auto_off_remain_time"] = "off", 0.0 // python-kasa offers auto-off only with these
		p.LatestFirmware = latest
	})
	s := run(t, "tapo-plug", "host.docker.internal", port(t, p.Addr()), sim.DemoUser, sim.DemoPassword)
	if get(s["energy"], "voltage") != 230.0 || get(s["energy"], "today") != 0.12 {
		t.Errorf("energy %v", s["energy"])
	}
	if get(s["latest-info"], "available") != true || get(s["latest-info"], "version") != latest {
		t.Errorf("latest %v", s["latest-info"])
	}
	if p.Get("fw_ver") != latest || get(s["device2"], "fw") != latest {
		t.Errorf("firmware after update %v, python-kasa %v", p.Get("fw_ver"), s["device2"])
	}
	if ao := p.Setting("auto_off_config"); ao["enable"] != true || ao["delay_min"] != 45.0 || get(s["device2"], "delay") != 45.0 {
		t.Errorf("auto off %v %v", ao, s["device2"])
	}
	if p.Setting("led_info")["led_rule"] != "never" || get(s["device2"], "led") != false {
		t.Errorf("led %v %v", p.Setting("led_info"), s["device2"])
	}
	if sc, _ := s["scan"].([]any); len(sc) != 2 || get(sc[0], "ssid") != "Demo-Home" {
		t.Errorf("scan %v", s["scan"])
	}
	if p.TapoReboots() != 1 {
		t.Errorf("reboots %d", p.TapoReboots())
	}
}

// python-kasa's Tapo bulb: colour, transition (version 3), a light effect
// and the presets.
func TestTapoBulbModules(t *testing.T) {
	needDocker(t)
	info := tapoInfo("L530", "3C-52-A1-00-00-05")
	info["type"], info["brightness"], info["hue"], info["saturation"], info["color_temp"] = "SMART.TAPOBULB", 50.0, 0.0, 100.0, 2700.0
	info["color_temp_range"] = []any{2500.0, 6500.0}
	info["dynamic_light_effect_enable"], info["dynamic_light_effect_id"] = false, ""
	b := sim.StartTapo(sim.DemoUser, sim.DemoPassword, info, nil)
	defer b.Close()
	b.Do(func() {
		b.Components = map[string]int{"brightness": 1, "color": 1, "color_temperature": 1, "on_off_gradually": 3, "light_effect": 1, "preset": 1}
		b.Presets = []map[string]any{{"brightness": 50.0, "hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}, {"brightness": 100.0, "hue": 240.0, "saturation": 100.0, "color_temp": 0.0}}
		b.Effects = []map[string]any{{"id": "L1", "scene_name": "", "change_mode": "direct", "change_time": 1000.0, "color_status_list": []any{[]any{100.0, 0.0, 0.0, 2700.0}, []any{100.0, 321.0, 99.0, 0.0}}},
			{"id": "L2", "scene_name": "", "change_mode": "bln", "change_time": 2000.0, "color_status_list": []any{[]any{100.0, 54.0, 6.0, 0.0}, []any{100.0, 19.0, 39.0, 0.0}}}}
	})
	s := run(t, "tapo-bulb", "host.docker.internal", port(t, b.Addr()), sim.DemoUser, sim.DemoPassword)
	if hsv, _ := get(s["device2"], "hsv").([]any); len(hsv) != 3 || hsv[0] != 120.0 || hsv[2] != 40.0 {
		t.Errorf("hsv %v", s["device2"])
	}
	if get(s["device2"], "on_transition") != 4.0 || b.Setting("on_off_gradually_info")["on_state"].(map[string]any)["duration"] != 4.0 {
		t.Errorf("transition %v %v", s["device2"], b.Setting("on_off_gradually_info"))
	}
	var firstBri any
	b.Do(func() { firstBri = b.Effects[0]["color_status_list"].([]any)[0].([]any)[0] })
	if b.Get("dynamic_light_effect_enable") != true || b.Get("dynamic_light_effect_id") != "L1" || get(s["device2"], "effect") != "Party" || firstBri == 100.0 { // python-kasa sets the bulb's brightness into the effect first
		t.Errorf("effect %v %v, python-kasa %v", b.Get("dynamic_light_effect_enable"), b.Get("dynamic_light_effect_id"), s["device2"])
	}
	if pl, _ := get(s["presets"], "list").([]any); len(pl) != 3 { // Not set, and the two presets
		t.Errorf("presets %v", s["presets"])
	}
}

// python-kasa's hub: a temperature and humidity sensor, a button with its
// log, and a contact sensor, through the simulated hub.
func TestTapoHubChildren(t *testing.T) {
	needDocker(t)
	info := tapoInfo("H100", "3C-52-A1-00-00-07")
	info["type"] = "SMART.TAPOHUB"
	h := sim.StartTapo(sim.DemoUser, sim.DemoPassword, info, nil)
	defer h.Close()
	child := func(id, model, cat string, extra map[string]any) map[string]any {
		m := map[string]any{"device_id": id, "model": model, "category": cat, "type": "SMART.TAPOSENSOR", "nickname": "U2Vuc29y", "status": "online",
			"fw_ver": "1.7.0 Build 230424", "hw_ver": "1.0", "mac": "F0A731000000", "parent_device_id": "80223F3C52A1000007", "at_low_battery": false,
			"rssi": -45.0, "signal_level": 3.0, "region": "Europe/London", "specs": "EU", "lastOnboardingTimestamp": 1706990901.0, "jamming_rssi": -118.0,
			"oem_id": "O1", "hw_id": "H1", "avatar": "", "bind_count": 1.0, "status_follow_edge": false}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	h.Do(func() {
		h.Children = []map[string]any{
			child("T1", "T315", "subg.trigger.temp-hmdt-sensor", map[string]any{"current_temp": 22.5, "current_humidity": 48.0, "temp_unit": "celsius",
				"current_temp_exception": 0.0, "current_humidity_exception": 0.0, "battery_percentage": 80.0, "report_interval": 16.0}),
			child("B1", "S200B", "subg.trigger.button", map[string]any{"report_interval": 16.0}),
			child("C1", "T110", "subg.trigger.contact-sensor", map[string]any{"open": true, "report_interval": 16.0}),
		}
	})
	h.Do(func() {
		h.ChildComponents = map[string]map[string]int{"C1": {"device": 2, "time": 1, "battery_detect": 1}}
	})
	h.Press("B1", "singleClick")
	s := run(t, "tapo-hub", "host.docker.internal", port(t, h.Addr()), sim.DemoUser, sim.DemoPassword)
	kids, _ := s["children"].(map[string]any)
	if len(kids) != 3 {
		t.Fatalf("children %v", s["children"])
	}
	if get(kids["T1"], "features", "temperature") != 22.5 || get(kids["T1"], "features", "humidity") != 48.0 {
		t.Errorf("T315 %v", kids["T1"])
	}
	if l, _ := get(kids["B1"], "logs").([]any); len(l) != 1 || l[0] != "singleClick" {
		t.Errorf("S200B %v", kids["B1"])
	}
	if get(kids["C1"], "features", "is_open") != true {
		t.Errorf("T110 %v", kids["C1"])
	}
}

// python-kasa decrypts the encrypted part of the simulated 20002 answer.
func TestTapoDiscoverySecret(t *testing.T) {
	needDocker(t)
	d := sim.StartTapo(sim.DemoUser, sim.DemoPassword, tapoInfo("P110", "3C-52-A1-00-00-04"), nil)
	defer d.Close()
	d.Do(func() { d.DiscoverySecret = map[string]any{"connect_ssid": "Demo-Home", "connect_type": "wireless"} })
	r, err := sim.StartResponder(d)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s := run(t, "tdp-secret", "host.docker.internal", port(t, r.Addr()), sim.DemoUser, sim.DemoPassword)
	found, _ := s["discover"].(map[string]any)
	if len(found) != 1 {
		t.Fatalf("discover %v", s["discover"])
	}
	for _, v := range found {
		if get(v, "connect_ssid") != "Demo-Home" {
			t.Errorf("decrypted %v", v)
		}
	}

}
