// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/cB-Sigma/tplink"
)

// TapoState is the state of the Tapo requests beyond switching and
// reading: settings, rules, light presets and effects, firmware, Wi-Fi.
// Set the exported fields before the first request.
type TapoState struct {
	// Components are reported by component_nego besides the device's own
	// (device, time, energy and children when present), by id with their
	// version code. A client such as python-kasa asks the requests of each
	// component it is told about.
	Components map[string]int
	// ChildComponents are the children's components
	// (get_child_device_component_list), by device_id; a child not listed
	// gets components from its category.
	ChildComponents map[string]map[string]int
	// Settings answer get_<name> and are merged by set_<name>, by name
	// (auto_off_config, led_info, on_off_gradually_info, auto_update_info,
	// lighting_effect, ...). Names not set answer the demo defaults.
	Settings map[string]map[string]any
	// Rules are the rule lists get_<kind>_rules reads, by kind (schedule,
	// countdown, antitheft).
	Rules map[string][]map[string]any
	// Presets are a light's saved states (get_preset_rules).
	Presets []map[string]any
	// Effects are a bulb's light effects (get_dynamic_light_effect_rules).
	Effects []map[string]any
	// LatestFirmware is the version get_latest_fw offers; "" or the
	// device's own fw_ver: none. fw_download installs it in three polls of
	// get_fw_download_state.
	LatestFirmware string
	// Networks are what get_wireless_scan_info finds (SSIDs in plain
	// text); nil: a demo list.
	Networks []map[string]any
	// DiscoverySecret, when set, is sent in the 20002 answer as
	// encrypt_info, encrypted to the probe's key as newer firmware does.
	DiscoverySecret map[string]any
	// CamSections are a camera's settings by section (StartCamera): a
	// get request naming a section answers it, a set request merges into
	// it; a section not listed is unsupported.
	CamSections map[string]map[string]any
	// PasswordMD5 makes a camera hold the MD5 hash of its password
	// (older firmware) instead of SHA-256.
	PasswordMD5 bool
	// PlainLogin makes a camera offer only the plain login (firmware
	// without encrypt_type 3), which then answers in clear.
	PlainLogin bool
	// CamState is a camera's motor, alarm and recordings (camera_more.go).
	CamState
	// Vacuum is a robot vacuum's state (vacuum.go); nil: not a vacuum.
	Vacuum *VacuumState

	ruleID    int // the last rule id given
	fwPolls   int
	fwRunning bool
	qs        map[string]any
	reboots   int
}

// Do runs f while the device is locked: change exported fields (Info,
// Children, TapoState) inside it once the device may be serving.
func (t *Tapo) Do(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f()
}

// QuickSetup is the last set_qs_info received (nil: none).
func (t *Tapo) QuickSetup() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return copyMap(t.qs)
}

// TapoReboots counts device_reboot and device_reset requests.
func (t *Tapo) TapoReboots() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reboots
}

// Setting returns a setting's current value (see Settings).
func (t *Tapo) Setting(name string) map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return copyMap(t.setting(name))
}

var settingDefaults = map[string]func() map[string]any{
	"auto_off_config": func() map[string]any { return map[string]any{"enable": false, "delay_min": 120.0} },
	"led_info": func() map[string]any {
		return map[string]any{"led_rule": "always", "led_status": true, "night_mode": map[string]any{"night_mode_type": "sunrise_sunset", "start_time": 1252.0, "end_time": 410.0, "sunrise_offset": 0.0, "sunset_offset": 0.0}}
	},
	"auto_update_info": func() map[string]any { return map[string]any{"enable": true, "time": 180.0, "random_range": 120.0} },
	"on_off_gradually_info": func() map[string]any {
		return map[string]any{"on_state": map[string]any{"enable": true, "duration": 2.0, "max_duration": 60.0}, "off_state": map[string]any{"enable": true, "duration": 2.0, "max_duration": 60.0}}
	},
	"lighting_effect": func() map[string]any {
		return map[string]any{"id": "", "name": "", "enable": 0.0, "brightness": 50.0, "custom": 0.0}
	},
	"connect_cloud_state": func() map[string]any { return map[string]any{"status": 0.0} },
	"inherit_info":        func() map[string]any { return map[string]any{"inherit_status": true} },
}

// setting returns the stored setting, creating it from the defaults. The
// caller holds t.mu.
func (t *Tapo) setting(name string) map[string]any {
	if s, ok := t.Settings[name]; ok {
		return s
	}
	def, ok := settingDefaults[name]
	if !ok {
		return nil
	}
	if t.Settings == nil {
		t.Settings = map[string]map[string]any{}
	}
	t.Settings[name] = def()
	return t.Settings[name]
}

// page answers a paged list as devices do (start_index, sum; ten per page).
func page(list []map[string]any, params map[string]any, key string, extra map[string]any) map[string]any {
	start := int(num(params["start_index"]))
	start = max(0, min(start, len(list)))
	end := min(start+10, len(list))
	out := []any{}
	for _, x := range list[start:end] {
		out = append(out, x)
	}
	res := map[string]any{key: out, "start_index": start, "sum": len(list)}
	for k, v := range extra {
		res[k] = v
	}
	return res
}

func smartOK(result map[string]any) map[string]any {
	if result == nil {
		result = map[string]any{}
	}
	return map[string]any{"error_code": 0, "result": result}
}

var smartErr = func(code int) map[string]any { return map[string]any{"error_code": code} }

// DemoHourWh is the simulated energy of an hour, in Wh.
func DemoHourWh(hour int) float64 { return float64(5 + hour%7) }

// energyData answers get_energy_data: hours from start to end, the days
// of start's quarter from start, or the twelve months of start's year.
func energyData(params map[string]any) map[string]any {
	start := time.Unix(int64(num(params["start_timestamp"])), 0).UTC()
	end := time.Unix(int64(num(params["end_timestamp"])), 0).UTC()
	interval := int(num(params["interval"]))
	data := []any{}
	switch interval {
	case 60:
		for h, at := 0, start; !at.After(end) && h < 24*31; h, at = h+1, at.Add(time.Hour) {
			data = append(data, DemoHourWh(at.Hour()))
		}
	case 1440:
		qEnd := time.Date(start.Year(), (start.Month()-1)/3*3+4, 1, 0, 0, 0, 0, time.UTC)
		for at := start; at.Before(qEnd); at = at.AddDate(0, 0, 1) {
			data = append(data, DemoDayWh(int(at.Month()), at.Day()))
		}
	case 43200:
		start = time.Date(start.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		for m := 1; m <= 12; m++ {
			sum := 0.0
			for d := 1; d <= days(start.Year(), m); d++ {
				sum += DemoDayWh(m, d)
			}
			data = append(data, sum)
		}
	default:
		return smartErr(-1008)
	}
	return smartOK(map[string]any{"data": data, "start_timestamp": start.Unix(), "end_timestamp": end.Unix(), "interval": interval,
		"local_time": time.Now().UTC().Format("2006-01-02 15:04:05")})
}

// smartMore answers the requests beyond switching and reading; done is
// false for a method it does not know. The caller holds t.mu.
func (t *Tapo) smartMore(method string, params map[string]any) (map[string]any, bool) {
	if params == nil {
		params = map[string]any{}
	}
	if t.Vacuum != nil {
		if r, ok := t.vacuum(method, params); ok {
			return r, true
		}
	}
	if r, ok := t.ruleWrite(method, params); ok {
		return r, true
	}
	switch method {
	case "get_energy_data":
		if t.Usage == nil {
			return smartErr(-1002), true
		}
		return energyData(params), true
	case "get_emeter_data":
		if t.Usage == nil {
			return smartErr(-1002), true
		}
		p := num(t.Usage["current_power"])
		return smartOK(map[string]any{"voltage_mv": 230000.0, "current_ma": p * 1000 / 230, "power_mw": p * 1000, "energy_wh": num(t.Usage["today_energy"])}), true
	case "get_emeter_vgain_igain":
		if t.Usage == nil {
			return smartErr(-1002), true
		}
		return smartOK(map[string]any{"vgain": 125000, "igain": 11000}), true
	case "edit_dynamic_light_effect_rule":
		for i, e := range t.Effects {
			if e["id"] == params["id"] {
				t.Effects[i] = copyMap(params)
				return smartOK(nil), true
			}
		}
		return smartErr(-1008), true
	case "get_device_usage":
		now := time.Now()
		r := map[string]any{"time_usage": map[string]any{"today": DemoDayMinutes(int(now.Month()), now.Day()), "past7": 600.0, "past30": 2400.0}}
		if t.Usage != nil {
			r["power_usage"] = map[string]any{"today": num(t.Usage["today_energy"]), "past7": 900.0, "past30": 3600.0}
			r["saved_power"] = map[string]any{"today": 0.0, "past7": 0.0, "past30": 0.0}
		}
		return smartOK(r), true
	case "get_latest_fw":
		cur, _ := t.Info["fw_ver"].(string)
		if t.LatestFirmware == "" || t.LatestFirmware == cur {
			return smartOK(map[string]any{"type": 0, "need_to_upgrade": false, "fw_ver": cur, "fw_size": 0, "release_date": "", "release_note": "", "oem_id": "", "hw_id": ""}), true
		}
		return smartOK(map[string]any{"type": 1, "need_to_upgrade": true, "fw_ver": t.LatestFirmware, "fw_size": 1048576, "release_date": "2026-10-01",
			"release_note": "Demo release.", "oem_id": "", "hw_id": ""}), true
	case "fw_download":
		cur, _ := t.Info["fw_ver"].(string)
		if t.LatestFirmware == "" || t.LatestFirmware == cur {
			return smartErr(-1001), true
		}
		t.fwRunning, t.fwPolls = true, 0
		return smartOK(nil), true
	case "get_fw_download_state":
		st := map[string]any{"status": 0, "download_progress": 0, "reboot_time": 5, "upgrade_time": 5, "auto_upgrade": false}
		if t.fwRunning {
			t.fwPolls++
			switch t.fwPolls {
			case 1:
				st["status"], st["download_progress"] = 2, 50
			case 2:
				st["status"], st["download_progress"], st["upgrade_time"] = 3, 100, 0
			default:
				t.fwRunning = false
				t.Info["fw_ver"] = t.LatestFirmware
			}
		}
		return smartOK(st), true
	case "get_schedule_rules", "get_countdown_rules", "get_antitheft_rules":
		kind := strings.TrimSuffix(strings.TrimPrefix(method, "get_"), "_rules")
		max := map[string]int{"schedule": 32, "countdown": 1, "antitheft": 1}[kind]
		return smartOK(page(t.Rules[kind], params, "rule_list", map[string]any{"enable": len(t.Rules[kind]) > 0, kind + "_rule_max_count": max})), true
	case "get_preset_rules":
		return smartOK(page(t.Presets, params, "states", nil)), true
	case "edit_preset_rules":
		i := int(num(params["index"]))
		st, _ := params["state"].(map[string]any)
		if i < 0 || i >= len(t.Presets) || st == nil {
			return smartErr(-1008), true
		}
		t.Presets[i] = st
		return smartOK(nil), true
	case "get_dynamic_light_effect_rules":
		cur, _ := t.Info["dynamic_light_effect_id"].(string)
		return smartOK(page(t.Effects, params, "rule_list", map[string]any{"enable": t.Info["dynamic_light_effect_enable"] == true, "max_count": 2,
			"current_rule_id": cur})), true
	case "set_dynamic_light_effect_rule_enable":
		on, _ := params["enable"].(bool)
		t.Info["dynamic_light_effect_enable"] = on
		if on {
			t.Info["dynamic_light_effect_id"] = params["id"]
		}
		return smartOK(nil), true
	case "get_wireless_scan_info":
		nets := t.Networks
		if nets == nil {
			nets = []map[string]any{{"ssid": "Demo-Home", "key_type": "wpa2_psk", "cipher_type": 2, "signal_level": 3, "channel": 6, "bssid": "000000000001"},
				{"ssid": "Demo-Guest", "key_type": "none", "cipher_type": 0, "signal_level": 1, "channel": 11, "bssid": "000000000002"}}
		}
		enc := make([]map[string]any, len(nets))
		for i, n := range nets {
			m := copyMap(n)
			s, _ := m["ssid"].(string)
			m["ssid"] = base64.StdEncoding.EncodeToString([]byte(s))
			enc[i] = m
		}
		return smartOK(page(enc, params, "ap_list", map[string]any{"wep_supported": false})), true
	case "set_qs_info":
		t.qs = copyMap(params)
		if acc, ok := params["account"].(map[string]any); ok && t.aes == nil && t.iot == nil {
			u, _ := base64.StdEncoding.DecodeString(str(acc["username"]))
			p, _ := base64.StdEncoding.DecodeString(str(acc["password"]))
			// The device takes the new account for later handshakes.
			if t.version == 1 {
				t.auth = tplink.AuthHashV1(string(u), string(p))
			} else {
				t.auth = tplink.AuthHash(string(u), string(p))
			}
		}
		return smartOK(nil), true
	case "device_reboot", "device_reset":
		t.reboots++
		return smartOK(nil), true
	case "get_child_device_component_list":
		if t.Children == nil {
			return smartErr(-1002), true
		}
		list := make([]map[string]any, len(t.Children))
		for i, c := range t.Children {
			id, _ := c["device_id"].(string)
			list[i] = map[string]any{"device_id": id, "component_list": componentList(t.childComponents(c))}
		}
		return smartOK(page(list, params, "child_component_list", nil)), true
	}
	if name, ok := strings.CutPrefix(method, "get_"); ok {
		if s := t.setting(name); s != nil {
			return smartOK(s), true
		}
	}
	if name, ok := strings.CutPrefix(method, "set_"); ok {
		if s := t.setting(name); s != nil {
			for k, v := range params {
				if sub, ok := v.(map[string]any); ok {
					if cur, ok := s[k].(map[string]any); ok {
						for kk, vv := range sub {
							cur[kk] = vv
						}
						continue
					}
				}
				s[k] = v
			}
			if name == "lighting_effect" {
				t.Info["lighting_effect"] = copyMap(s)
			}
			return smartOK(nil), true
		}
	}
	return nil, false
}

func str(v any) string { s, _ := v.(string); return s }

func componentList(m map[string]int) []any {
	out := []any{}
	for _, id := range sortedKeys(m) {
		out = append(out, map[string]any{"id": id, "ver_code": m[id]})
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// childComponents are a child's components: listed in ChildComponents,
// else from its category.
func (t *Tapo) childComponents(c map[string]any) map[string]int {
	id, _ := c["device_id"].(string)
	if m, ok := t.ChildComponents[id]; ok {
		return m
	}
	m := map[string]int{"device": 2, "time": 1}
	cat, _ := c["category"].(string)
	switch {
	case strings.Contains(cat, "temp-hmdt"):
		m["temperature"], m["humidity"], m["battery_detect"] = 1, 1, 1
	case strings.Contains(cat, "button"):
		m["trigger_log"], m["battery_detect"] = 1, 1
	case strings.Contains(cat, "sub-plug"):
		m["countdown"] = 2
	}
	return m
}
