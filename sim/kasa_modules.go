// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"fmt"
	"time"
)

// Kasa module names differ between plugs and bulbs, and devices disagree
// on the countdown module's name; the simulator answers each under every
// name.
var kasaAliases = map[string]string{
	"smartlife.iot.common.emeter":           "emeter",
	"smartlife.iot.common.schedule":         "schedule",
	"smartlife.iot.common.anti_theft":       "anti_theft",
	"smartlife.iot.common.cloud":            "cnCloud",
	"smartlife.iot.common.timesetting":      "time",
	"smartlife.iot.common.system":           "system",
	"smartlife.iot.common.softaponboarding": "netif",
	"countdown":                             "count_down",
}

func canonical(mod string) string {
	if c, ok := kasaAliases[mod]; ok {
		return c
	}
	return mod
}

// KasaState is the state of the modules beyond switching and reading.
type KasaState struct {
	rules         map[string]*ruleSet // schedule, count_down, anti_theft
	ruleID        int
	dimmer        map[string]any
	joined        map[string]any
	reboots       int
	energyErased  bool
	runtimeErased bool
	// Networks are what a Wi-Fi scan finds (get_scaninfo); nil: a demo
	// list.
	Networks []map[string]any
	// Firmware is the cloud's firmware list (get_intl_fw_list); nil: none.
	Firmware []map[string]any
	// AnyLogin makes a Kasa camera (StartKasaCamera) take any user and
	// password, for clients that send their own default.
	AnyLogin bool
}

type ruleSet struct {
	enable bool
	list   []map[string]any
}

// Rules returns a rule module's rules (module schedule, count_down or
// anti_theft).
func (k *Kasa) Rules(module string) []map[string]any {
	k.mu.Lock()
	defer k.mu.Unlock()
	rs := k.rules[canonical(module)]
	if rs == nil {
		return nil
	}
	out := make([]map[string]any, len(rs.list))
	for i, r := range rs.list {
		out[i] = copyMap(r)
	}
	return out
}

// Do runs f while the device is locked: change exported fields
// (Sysinfo, Emeter, KasaState) inside it once the device may be serving.
func (k *Kasa) Do(f func()) {
	k.mu.Lock()
	defer k.mu.Unlock()
	f()
}

// Joined is the last network a set_stainfo asked to join (nil: none).
func (k *Kasa) Joined() map[string]any {
	k.mu.Lock()
	defer k.mu.Unlock()
	return copyMap(k.joined)
}

// Reboots counts reboot requests.
func (k *Kasa) Reboots() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.reboots
}

// DimmerSetting returns a dimmer setting (fadeOnTime, fadeOffTime,
// gentleOnTime, gentleOffTime, rampRate, minThreshold).
func (k *Kasa) DimmerSetting(key string) any {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.dimmerParams()[key]
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func okAnswer(m map[string]any) map[string]any {
	out := copyMap(m)
	if out == nil {
		out = map[string]any{}
	}
	out["err_code"] = 0.0
	return out
}

var (
	errModule = map[string]any{"err_code": -1.0, "err_msg": "module not support"}
	errParam  = map[string]any{"err_code": -3.0, "err_msg": "invalid argument"}
	errEntry  = map[string]any{"err_code": -14.0, "err_msg": "entry not exist"}
)

// DemoDayWh is the simulated energy of a day, in Wh: made-up figures that
// differ by day and month.
func DemoDayWh(month, day int) float64 { return float64(100 + 7*day + month) }

// DemoDayMinutes is the simulated time on of a day, in minutes.
func DemoDayMinutes(month, day int) float64 { return float64(60 + day + month) }

// days are the days of a month that have passed, by the device clock.
func days(year, month int) int {
	now := time.Now()
	switch {
	case year > now.Year() || (year == now.Year() && month > int(now.Month())) || month < 1 || month > 12:
		return 0
	case year == now.Year() && month == int(now.Month()):
		return now.Day()
	}
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// stats answers get_daystat and get_monthstat from per-day figures.
func stats(meth string, a map[string]any, key string, perDay func(m, d int) float64, scale float64) map[string]any {
	year := int(num(a["year"]))
	if year == 0 {
		return errParam
	}
	if meth == "get_daystat" {
		month := int(num(a["month"]))
		list := []any{}
		for d := 1; d <= days(year, month); d++ {
			list = append(list, map[string]any{"year": float64(year), "month": float64(month), "day": float64(d), key: perDay(month, d) * scale})
		}
		return okAnswer(map[string]any{"day_list": list})
	}
	list := []any{}
	for m := 1; m <= 12; m++ {
		n := days(year, m)
		if n == 0 {
			continue
		}
		sum := 0.0
		for d := 1; d <= n; d++ {
			sum += perDay(m, d)
		}
		list = append(list, map[string]any{"year": float64(year), "month": float64(m), key: sum * scale})
	}
	return okAnswer(map[string]any{"month_list": list})
}

func (k *Kasa) dimmerParams() map[string]any {
	if k.dimmer == nil {
		k.dimmer = map[string]any{"minThreshold": 1.0, "fadeOnTime": 1000.0, "fadeOffTime": 1000.0, "gentleOnTime": 3000.0,
			"gentleOffTime": 10000.0, "rampRate": 30.0, "bulb_type": 1.0}
	}
	return k.dimmer
}

// module answers the modules beyond switching and reading; done is false
// for a method it leaves to handle. The caller holds k.mu.
func (k *Kasa) module(mod, meth string, a map[string]any, childIDs []any) (map[string]any, bool) {
	if a == nil {
		a = map[string]any{}
	}
	switch mod {
	case "system":
		switch meth {
		case "set_dev_alias":
			if alias, ok := a["alias"].(string); ok {
				if len(childIDs) > 0 {
					k.eachChild(childIDs, func(c map[string]any) { c["alias"] = alias })
				} else {
					k.Sysinfo["alias"] = alias
				}
				return okAnswer(nil), true
			}
			return errParam, true
		case "set_led_off":
			k.Sysinfo["led_off"] = num(a["off"])
			return okAnswer(nil), true
		case "reboot", "reset":
			k.reboots++
			return okAnswer(nil), true
		}
	case "emeter":
		if meth == "get_realtime" {
			return nil, false
		}
		if k.Emeter == nil && k.ChildEmeter == nil {
			return errModule, true
		}
		switch meth {
		case "get_daystat", "get_monthstat":
			if k.energyErased {
				return stats(meth, a, "energy_wh", func(int, int) float64 { return 0 }, 1), true
			}
			return stats(meth, a, "energy_wh", DemoDayWh, 1), true
		case "erase_emeter_stat":
			k.energyErased = true
			return okAnswer(nil), true
		}
	case "schedule", "count_down", "anti_theft":
		if mod == "schedule" {
			switch meth {
			case "get_daystat", "get_monthstat":
				if k.runtimeErased {
					return stats(meth, a, "time", func(int, int) float64 { return 0 }, 1), true
				}
				return stats(meth, a, "time", DemoDayMinutes, 1), true
			case "erase_runtime_stat":
				k.runtimeErased = true
				return okAnswer(nil), true
			case "get_next_action":
				return okAnswer(map[string]any{"type": -1.0}), true
			}
		}
		return k.ruleMethod(mod, meth, a)
	case "smartlife.iot.dimmer":
		p := k.dimmerParams()
		switch meth {
		case "get_dimmer_parameters":
			return okAnswer(p), true
		case "get_default_behavior":
			return okAnswer(map[string]any{"double_click": map[string]any{"mode": "none"}, "long_press": map[string]any{"mode": "instant_on_off"},
				"hard_on": map[string]any{"mode": "last_status"}, "soft_on": map[string]any{"mode": "last_status"}}), true
		case "set_dimmer_transition":
			b, ok := a["brightness"].(float64)
			if !ok || b < 0 || b > 100 {
				return errParam, true
			}
			if b == 0 {
				k.Sysinfo["relay_state"] = 0.0
			} else {
				k.Sysinfo["brightness"], k.Sysinfo["relay_state"] = b, 1.0
			}
			return okAnswer(nil), true
		case "set_fade_on_time", "set_fade_off_time":
			key := map[string]string{"set_fade_on_time": "fadeOnTime", "set_fade_off_time": "fadeOffTime"}[meth]
			p[key] = num(a["fadeTime"])
			return okAnswer(nil), true
		case "set_gentle_on_time", "set_gentle_off_time":
			key := map[string]string{"set_gentle_on_time": "gentleOnTime", "set_gentle_off_time": "gentleOffTime"}[meth]
			p[key] = num(a["duration"])
			return okAnswer(nil), true
		case "set_button_ramp_rate":
			p["rampRate"] = num(a["rampRate"])
			return okAnswer(nil), true
		case "calibrate_brightness":
			p["minThreshold"] = num(a["minThreshold"])
			return okAnswer(nil), true
		}
	case "smartlife.iot.smartbulb.lightingservice", "smartlife.iot.lightStrip":
		_, strip := k.Sysinfo["length"]
		if strip != (mod == "smartlife.iot.lightStrip") {
			return nil, false // handle answers the module error
		}
		ls, _ := k.Sysinfo["light_state"].(map[string]any)
		switch meth {
		case "get_light_state":
			return okAnswer(ls), true
		case "get_light_details":
			return okAnswer(map[string]any{"lamp_beam_angle": 150.0, "min_voltage": 110.0, "max_voltage": 120.0, "wattage": 10.0,
				"incandescent_equivalent": 60.0, "max_lumens": 800.0, "color_rendering_index": 80.0}), true
		case "get_default_behavior":
			return okAnswer(map[string]any{"hard_on": map[string]any{"mode": "last_status"}, "soft_on": map[string]any{"mode": "last_status"}}), true
		case "set_preferred_state":
			idx := int(num(a["index"]))
			pref, _ := k.Sysinfo["preferred_state"].([]any)
			for len(pref) <= idx {
				pref = append(pref, map[string]any{"index": float64(len(pref))})
			}
			pref[idx] = copyMap(a)
			k.Sysinfo["preferred_state"] = pref
			return okAnswer(nil), true
		}
	case "smartlife.iot.lighting_effect":
		if _, strip := k.Sysinfo["length"]; !strip {
			return errModule, true
		}
		if meth == "set_lighting_effect" {
			st, _ := k.Sysinfo["lighting_effect_state"].(map[string]any)
			if st == nil {
				st = map[string]any{}
			}
			for _, key := range []string{"enable", "name", "id", "brightness", "custom"} {
				if v, ok := a[key]; ok {
					st[key] = v
				}
			}
			k.Sysinfo["lighting_effect_state"] = st
			return okAnswer(nil), true
		}
	case "cnCloud":
		switch meth {
		case "get_info":
			return okAnswer(map[string]any{"username": "", "server": "devs.tplinkcloud.com", "binded": 0.0, "cld_connection": 0.0,
				"illegalType": 0.0, "stopConnect": 0.0, "tcspStatus": 0.0, "fwDlPage": "", "tcspInfo": "", "fwNotifyType": 0.0}), true
		case "get_intl_fw_list":
			list := []any{}
			for _, f := range k.Firmware {
				list = append(list, f)
			}
			return okAnswer(map[string]any{"fw_list": list}), true
		}
	case "netif":
		switch meth {
		case "get_scaninfo":
			nets := k.Networks
			if nets == nil {
				nets = []map[string]any{{"ssid": "Demo-Home", "key_type": 3.0, "rssi": -48.0}, {"ssid": "Demo-Guest", "key_type": 0.0, "rssi": -71.0}}
			}
			list := make([]any, len(nets))
			for i, n := range nets {
				list[i] = n
			}
			return okAnswer(map[string]any{"ap_list": list, "wpa3_support": 0.0}), true
		case "set_stainfo":
			if _, ok := a["ssid"].(string); !ok {
				return errParam, true
			}
			k.joined = copyMap(a)
			return okAnswer(nil), true
		}
	}
	return nil, false
}

func (k *Kasa) eachChild(ids []any, f func(map[string]any)) {
	ch, _ := k.Sysinfo["children"].([]any)
	for _, c := range ch {
		cm, _ := c.(map[string]any)
		for _, id := range ids {
			if cm != nil && cm["id"] == id {
				f(cm)
			}
		}
	}
}

// ruleMethod answers the rule methods of schedule, count_down and
// anti_theft.
func (k *Kasa) ruleMethod(mod, meth string, a map[string]any) (map[string]any, bool) {
	if k.rules == nil {
		k.rules = map[string]*ruleSet{}
	}
	rs := k.rules[mod]
	if rs == nil {
		rs = &ruleSet{enable: true}
		k.rules[mod] = rs
	}
	find := func(id any) int {
		for i, r := range rs.list {
			if r["id"] == id {
				return i
			}
		}
		return -1
	}
	switch meth {
	case "get_rules":
		list := make([]any, len(rs.list))
		for i, r := range rs.list {
			list[i] = r
		}
		return okAnswer(map[string]any{"rule_list": list, "enable": float64(b2f(rs.enable)), "version": 2.0}), true
	case "add_rule":
		if mod == "count_down" && len(rs.list) > 0 {
			return map[string]any{"err_code": -10.0, "err_msg": "table is full"}, true
		}
		if _, ok := a["name"].(string); !ok {
			return errParam, true
		}
		k.ruleID++
		r := copyMap(a)
		r["id"] = fmt.Sprintf("SIMRULE%04d", k.ruleID)
		if mod == "count_down" {
			r["remain"] = num(a["delay"])
		}
		rs.list = append(rs.list, r)
		return okAnswer(map[string]any{"id": r["id"]}), true
	case "edit_rule":
		i := find(a["id"])
		if i < 0 {
			return errEntry, true
		}
		rs.list[i] = copyMap(a)
		return okAnswer(nil), true
	case "delete_rule":
		i := find(a["id"])
		if i < 0 {
			return errEntry, true
		}
		rs.list = append(rs.list[:i], rs.list[i+1:]...)
		return okAnswer(nil), true
	case "delete_all_rules":
		rs.list = nil
		return okAnswer(nil), true
	case "set_overall_enable":
		rs.enable = num(a["enable"]) == 1
		return okAnswer(nil), true
	}
	return nil, false
}

func b2f(b bool) int {
	if b {
		return 1
	}
	return 0
}
