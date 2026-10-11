// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

func klap(t *testing.T, d *sim.Tapo) *tplink.KLAPClient {
	t.Helper()
	return &tplink.KLAPClient{Base: "http://" + d.Addr(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// A Kasa plug with an energy meter: statistics, rules, LED, name, Wi-Fi.
func TestKasaPlugModules(t *testing.T) {
	k, err := sim.StartKasa(map[string]any{"alias": "Kettle", "model": "HS110(EU)", "mic_type": "IOT.SMARTPLUGSWITCH", "relay_state": 0.0},
		map[string]any{"voltage_mv": 230000.0, "current_ma": 100.0, "power_mw": 23000.0, "total_wh": 1200.0})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	d, info, err := tplink.NewKasaDevice(ctx, tplink.KasaConn{Addr: k.Addr()})
	if err != nil || d.Bulb || info["alias"] != "Kettle" {
		t.Fatalf("device %+v %v %v", d, info, err)
	}
	if e, err := d.Emeter(ctx); err != nil || e.PowerMW != 23000 {
		t.Errorf("emeter %+v %v", e, err)
	}
	now := time.Now()
	days, err := d.EmeterDays(ctx, now.Year(), int(now.Month()))
	if err != nil || len(days) != now.Day() || days[0].Value != sim.DemoDayWh(int(now.Month()), 1) {
		t.Errorf("days %v %v", days, err)
	}
	months, err := d.EmeterMonths(ctx, now.Year())
	if err != nil || len(months) != int(now.Month()) {
		t.Errorf("months %v %v", months, err)
	}
	if rt, err := d.RuntimeDays(ctx, now.Year(), int(now.Month())); err != nil || len(rt) != now.Day() || rt[0].Value != sim.DemoDayMinutes(int(now.Month()), 1) {
		t.Errorf("runtime %v %v", rt, err)
	}
	if err := d.EraseEmeter(ctx); err != nil {
		t.Fatal(err)
	}
	if days, _ := d.EmeterDays(ctx, now.Year(), int(now.Month())); len(days) == 0 || days[0].Value != 0 {
		t.Errorf("after erase %v", days)
	}

	// Schedule: add, edit, read, delete.
	r := tplink.KasaRule{Name: "morning", Enable: true, Repeat: true, SAction: tplink.KasaActionOn, STime: tplink.KasaTimeMinutes, SMin: 420, EAction: -1, ETime: -1}
	r.WDay = [7]bool{false, true, true, true, true, true, false}
	id, err := d.AddRule(ctx, tplink.KasaSchedule, r)
	if err != nil || id == "" {
		t.Fatalf("add %q %v", id, err)
	}
	r.ID, r.SMin = id, 450
	if err := d.EditRule(ctx, tplink.KasaSchedule, r); err != nil {
		t.Fatal(err)
	}
	rules, err := d.Rules(ctx, tplink.KasaSchedule)
	if err != nil || len(rules.Rules) != 1 || rules.Rules[0].SMin != 450 || !rules.Rules[0].WDay[1] || rules.Rules[0].WDay[0] || !rules.Enable {
		t.Fatalf("rules %+v %v", rules, err)
	}
	if err := d.DeleteRule(ctx, tplink.KasaSchedule, "nope"); err == nil {
		t.Error("deleting a missing rule passed")
	}
	if err := d.DeleteRule(ctx, tplink.KasaSchedule, id); err != nil || len(k.Rules("schedule")) != 0 {
		t.Errorf("delete %v %v", err, k.Rules("schedule"))
	}
	// Countdown and away mode.
	if _, err := d.AddRule(ctx, tplink.KasaCountdown, tplink.KasaRule{Name: "off", Enable: true, Delay: 600, Action: tplink.KasaActionOff}); err != nil {
		t.Fatal(err)
	}
	if cd := k.Rules("count_down"); len(cd) != 1 || cd[0]["delay"] != 600.0 {
		t.Errorf("countdown %v", cd)
	}
	if err := d.SetRulesEnabled(ctx, tplink.KasaAntiTheft, false); err != nil {
		t.Fatal(err)
	}
	if at, err := d.Rules(ctx, tplink.KasaAntiTheft); err != nil || at.Enable {
		t.Errorf("away mode %+v %v", at, err)
	}
	if err := d.DeleteAllRules(ctx, tplink.KasaCountdown); err != nil || len(k.Rules("count_down")) != 0 {
		t.Errorf("delete all %v", err)
	}

	if err := d.SetLED(ctx, false); err != nil || k.Get("led_off") != 1.0 {
		t.Errorf("LED %v %v", err, k.Get("led_off"))
	}
	if err := d.SetAlias(ctx, "Kitchen kettle"); err != nil || k.Get("alias") != "Kitchen kettle" {
		t.Errorf("alias %v", err)
	}
	if err := d.Reboot(ctx, 1); err != nil || k.Reboots() != 1 {
		t.Errorf("reboot %v", err)
	}
	nets, err := d.WifiScan(ctx)
	if err != nil || len(nets) != 2 || nets[0].SSID != "Demo-Home" || nets[0].KeyType != "3" {
		t.Errorf("scan %+v %v", nets, err)
	}
	if err := d.WifiJoin(ctx, "Demo-Home", "wifi-password", 3); err != nil || k.Joined()["ssid"] != "Demo-Home" {
		t.Errorf("join %v %v", err, k.Joined())
	}
	if ci, err := d.CloudInfo(ctx); err != nil || ci["binded"] != 0.0 {
		t.Errorf("cloud %v %v", ci, err)
	}
	if fw, err := d.FirmwareList(ctx); err != nil || len(fw) != 0 {
		t.Errorf("firmware %v %v", fw, err)
	}
}

// A strip's outlets are addressed with child_ids.
func TestKasaStripChild(t *testing.T) {
	k, err := sim.StartKasa(map[string]any{"alias": "Strip", "mic_type": "IOT.SMARTPLUGSWITCH", "children": []any{
		map[string]any{"id": "S00", "state": 0.0, "alias": "A"}, map[string]any{"id": "S01", "state": 0.0, "alias": "B"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	d := (&tplink.KasaDevice{Caller: tplink.KasaConn{Addr: k.Addr()}}).Child("S01")
	if err := d.SetRelay(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := d.SetAlias(ctx, "Lamp"); err != nil {
		t.Fatal(err)
	}
	kids := k.Get("children").([]any)
	if b := kids[1].(map[string]any); b["state"] != 1.0 || b["alias"] != "Lamp" || kids[0].(map[string]any)["alias"] != "A" {
		t.Errorf("children %v", kids)
	}
}

// A dimmer on KLAP v1 (Kasa JSON inside): transitions and fade times.
func TestKasaDimmerOnKLAP(t *testing.T) {
	dev, k := sim.StartKasaKLAP(sim.DemoUser, sim.DemoPassword, 1, map[string]any{"alias": "Hall", "model": "HS220(US)", "mic_type": "IOT.SMARTPLUGSWITCH",
		"relay_state": 0.0, "brightness": 10.0}, nil)
	defer dev.Close()
	d, _, err := tplink.NewKasaDevice(ctx, klap(t, dev))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DimTo(ctx, 80, 1500); err != nil || k.Get("brightness") != 80.0 || k.Get("relay_state") != 1.0 {
		t.Errorf("dim %v %v %v", err, k.Get("brightness"), k.Get("relay_state"))
	}
	if err := d.DimTo(ctx, 0, 500); err != nil || k.Get("relay_state") != 0.0 {
		t.Errorf("fade off %v", err)
	}
	if err := d.DimTo(ctx, 101, 0); err == nil {
		t.Error("brightness 101 accepted")
	}
	if err := d.SetDimmerFade(ctx, tplink.GentleOff, 20000); err != nil || k.DimmerSetting("gentleOffTime") != 20000.0 {
		t.Errorf("gentle off %v", err)
	}
	if err := d.SetRampRate(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if p, err := d.DimmerParameters(ctx); err != nil || p["rampRate"] != 20.0 || p["gentleOffTime"] != 20000.0 {
		t.Errorf("parameters %v %v", p, err)
	}
}

// A bulb and a light strip: light state with a transition, an effect.
func TestKasaLights(t *testing.T) {
	b, err := sim.StartKasa(map[string]any{"alias": "Bulb", "model": "KL130(EU)", "mic_type": "IOT.SMARTBULB",
		"light_state": map[string]any{"on_off": 0.0, "brightness": 50.0, "hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}}, map[string]any{"power_mw": 9000.0})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d, _, err := tplink.NewKasaDevice(ctx, tplink.KasaConn{Addr: b.Addr()})
	if err != nil || !d.Bulb || d.LightStrip {
		t.Fatalf("bulb %+v %v", d, err)
	}
	hue, sat := 240, 100
	ls, err := d.SetLight(ctx, tplink.KasaLight{Hue: &hue, Saturation: &sat, TransitionMS: 1000})
	if err != nil || ls["hue"] != 240.0 || ls["color_temp"] != 0.0 || ls["on_off"] != 1.0 {
		t.Errorf("light %v %v", ls, err)
	}
	if e, err := d.Emeter(ctx); err != nil || e.PowerMW != 9000 {
		t.Errorf("bulb emeter %+v %v", e, err)
	}
	if _, err := d.AddRule(ctx, tplink.KasaSchedule, tplink.KasaRule{Name: "dusk", Enable: true, SAction: 1, STime: tplink.KasaTimeSunset, EAction: -1, ETime: -1,
		Light: map[string]any{"brightness": 30}}); err != nil || len(b.Rules("smartlife.iot.common.schedule")) != 1 {
		t.Errorf("bulb schedule %v", err)
	}

	s, err := sim.StartKasa(map[string]any{"alias": "Strip", "model": "KL430(US)", "mic_type": "IOT.SMARTBULB", "length": 16.0,
		"light_state": map[string]any{"on_off": 1.0, "brightness": 50.0, "hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ds, _, err := tplink.NewKasaDevice(ctx, tplink.KasaConn{Addr: s.Addr()})
	if err != nil || !ds.LightStrip {
		t.Fatalf("strip %+v %v", ds, err)
	}
	bri := 20
	if ls, err := ds.SetLight(ctx, tplink.KasaLight{Brightness: &bri}); err != nil || ls["brightness"] != 20.0 {
		t.Errorf("strip light %v %v", ls, err)
	}
	if err := ds.SetLightingEffect(ctx, map[string]any{"name": "Demo", "id": "demo1", "enable": 1, "brightness": 60, "type": "sequence"}); err != nil {
		t.Fatal(err)
	}
	if st := s.Get("lighting_effect_state").(map[string]any); st["name"] != "Demo" || st["enable"] != 1.0 {
		t.Errorf("effect %v", st)
	}
	if err := d.SetLightingEffect(ctx, map[string]any{"enable": 0}); !tplink.IsKasaUnsupported(err) {
		t.Errorf("an effect on a bulb: %v", err)
	}
}

// Tapo batching, components and the typed helpers.
func TestTapoPlugHelpers(t *testing.T) {
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "P110", "type": "SMART.TAPOPLUG", "device_on": false, "fw_ver": "1.3.0 Build 230905",
		"nickname": "UGx1Zw=="}, map[string]any{"current_power": 12.0, "today_energy": 300.0, "month_energy": 4000.0, "today_runtime": 60.0, "month_runtime": 900.0})
	defer p.Close()
	p.Components = map[string]int{"firmware": 2, "auto_off": 1, "led": 1}
	p.LatestFirmware = "1.4.0 Build 260901"
	p.Rules = map[string][]map[string]any{"schedule": {{"id": "S1", "enable": true}}}
	c := klap(t, p)
	d := tplink.TapoDevice{Caller: c}

	res, err := tplink.MultiCall(ctx, c, []tplink.SmartRequest{{Method: "get_device_info"}, {Method: "get_energy_usage"}, {Method: "no_such"}, {Method: "get_current_power"}}, 0)
	if err != nil || len(res) != 4 || res[0].Result["model"] != "P110" || !tplink.IsUnsupported(res[2].Err) || res[3].Result["current_power"] != 12.0 {
		t.Fatalf("multi %+v %v", res, err)
	}
	comp, err := tplink.Components(ctx, c)
	if err != nil || comp["firmware"] != 2 || comp["energy_monitoring"] != 2 {
		t.Errorf("components %v %v", comp, err)
	}
	info, err := d.Info(ctx)
	if err != nil || info.Nickname != "Plug" || info.On == nil || *info.On {
		t.Errorf("info %+v %v", info, err)
	}
	if err := d.SetNickname(ctx, "Heater"); err != nil || p.Get("nickname") != "SGVhdGVy" {
		t.Errorf("nickname %v", err)
	}
	if u, err := d.EnergyUsage(ctx); err != nil || u.TodayEnergy != 300 || u.MonthRuntime != 900 {
		t.Errorf("usage %+v %v", u, err)
	}
	if w, err := d.CurrentPower(ctx); err != nil || w != 12 {
		t.Errorf("power %v %v", w, err)
	}
	if e, err := d.Emeter(ctx); err != nil || e.PowerMW != 12000 {
		t.Errorf("emeter %+v %v", e, err)
	}
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if h, err := d.EnergyData(ctx, day, day.Add(23*time.Hour), tplink.EnergyHourly); err != nil || len(h.Wh) != 24 || h.Wh[3] != sim.DemoHourWh(3) {
		t.Errorf("hourly %+v %v", h, err)
	}
	if dd, err := d.EnergyData(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), day, tplink.EnergyDaily); err != nil || len(dd.Wh) != 90 {
		t.Errorf("daily %d %v", len(dd.Wh), err)
	}
	if du, err := d.Usage(ctx); err != nil || du.Power.Today != 300 {
		t.Errorf("device usage %+v %v", du, err)
	}

	// Firmware: offered, downloaded, flashed.
	fw, err := d.LatestFirmware(ctx)
	if err != nil || !fw.Available() || fw.Version != "1.4.0 Build 260901" {
		t.Fatalf("latest %+v %v", fw, err)
	}
	if err := d.StartFirmwareUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	var states []int
	for i := 0; i < 3; i++ {
		st, err := d.FirmwareState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, st.Status)
	}
	if states[0] != 2 || states[1] != 3 || states[2] != 0 || p.Get("fw_ver") != "1.4.0 Build 260901" {
		t.Errorf("states %v, fw %v", states, p.Get("fw_ver"))
	}
	if fw, _ := d.LatestFirmware(ctx); fw.Available() {
		t.Error("an update is still offered after it was installed")
	}
	if err := d.SetAutoUpdate(ctx, tplink.TapoAutoUpdate{Enable: false, Time: 120, RandomRange: 30}); err != nil {
		t.Fatal(err)
	}
	if au, err := d.AutoUpdate(ctx); err != nil || au.Enable || au.Time != 120 {
		t.Errorf("auto update %+v %v", au, err)
	}

	if err := d.SetAutoOff(ctx, tplink.TapoAutoOff{Enable: true, DelayMin: 30}); err != nil {
		t.Fatal(err)
	}
	if ao, err := d.AutoOff(ctx); err != nil || !ao.Enable || ao.DelayMin != 30 {
		t.Errorf("auto off %+v %v", ao, err)
	}
	if err := d.SetLEDRule(ctx, "never"); err != nil || p.Setting("led_info")["led_rule"] != "never" || p.Setting("led_info")["night_mode"] == nil {
		t.Errorf("LED %v %v", err, p.Setting("led_info"))
	}
	if r, err := d.Rules(ctx, tplink.TapoSchedule); err != nil || len(r.Rules) != 1 || r.Max != 32 {
		t.Errorf("rules %+v %v", r, err)
	}
	if r, err := d.Rules(ctx, tplink.TapoCountdown); err != nil || len(r.Rules) != 0 || r.Max != 1 {
		t.Errorf("countdown %+v %v", r, err)
	}
	if err := d.Reboot(ctx, 1); err != nil || p.TapoReboots() != 1 {
		t.Errorf("reboot %v", err)
	}
}

// Set-up: Wi-Fi scan and join with the account, then the new account.
func TestTapoSetup(t *testing.T) {
	p := sim.StartTapo("", "", map[string]any{"model": "P110", "type": "SMART.TAPOPLUG"}, nil) // a reset device: blank account
	defer p.Close()
	d := tplink.TapoDevice{Caller: klap(t, p)}
	nets, err := d.WifiScan(ctx)
	if err != nil || len(nets) != 2 || nets[0].SSID != "Demo-Home" || nets[0].KeyType != "wpa2_psk" {
		t.Fatalf("scan %+v %v", nets, err)
	}
	if err := d.WifiJoin(ctx, sim.DemoUser, sim.DemoPassword, "Demo-Home", "wifi-password", nets[0].KeyType); err != nil {
		t.Fatal(err)
	}
	qs := p.QuickSetup()
	if w := qs["wireless"].(map[string]any); w["ssid"] != "RGVtby1Ib21l" || qs["time"] == nil {
		t.Errorf("set_qs_info %v", qs)
	}
	// The device now knows the account and refuses blank credentials.
	blank := &tplink.KLAPClient{Base: "http://" + p.Addr(), Auth: tplink.AuthHash("", ""), HTTP: http.DefaultClient}
	if err := blank.Handshake(ctx); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("blank after set-up: %v", err)
	}
	if _, err := (tplink.TapoDevice{Caller: klap(t, p)}).Info(ctx); err != nil {
		t.Errorf("the account after set-up: %v", err)
	}
}

// Bulbs and light strips on Tapo: light, transitions, presets, effects.
func TestTapoLights(t *testing.T) {
	b := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "L530", "type": "SMART.TAPOBULB", "device_on": false, "brightness": 50.0,
		"hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}, nil)
	defer b.Close()
	b.Presets = []map[string]any{{"brightness": 50.0, "hue": 0.0, "saturation": 0.0, "color_temp": 2700.0}}
	b.Effects = []map[string]any{{"id": "L1", "scene_name": ""}, {"id": "L2", "scene_name": ""}}
	d := tplink.TapoDevice{Caller: klap(t, b)}
	on, hue, sat := true, 120, 80
	if err := d.SetLight(ctx, tplink.TapoLight{On: &on, Hue: &hue, Saturation: &sat}); err != nil {
		t.Fatal(err)
	}
	if b.Get("hue") != 120.0 || b.Get("color_temp") != 0.0 || b.Get("device_on") != true {
		t.Errorf("light %v %v", b.Get("hue"), b.Get("color_temp"))
	}
	if err := d.SetTransitions(ctx, 2, true, 5, 3); err != nil {
		t.Fatal(err)
	}
	if tr, err := d.Transitions(ctx); err != nil || tr["on_state"].(map[string]any)["duration"] != 5.0 || tr["off_state"].(map[string]any)["max_duration"] != 60.0 {
		t.Errorf("transitions %v %v", tr, err)
	}
	if err := d.EditPreset(ctx, 0, map[string]any{"brightness": 10, "color_temp": 4000}); err != nil {
		t.Fatal(err)
	}
	if ps, err := d.Presets(ctx); err != nil || len(ps) != 1 || ps[0]["brightness"] != 10.0 {
		t.Errorf("presets %v %v", ps, err)
	}
	if err := d.SetDynamicEffect(ctx, "L2", true); err != nil || b.Get("dynamic_light_effect_id") != "L2" {
		t.Errorf("effect %v", err)
	}
	if ef, err := d.DynamicEffects(ctx); err != nil || !ef.Enable || len(ef.Rules) != 2 {
		t.Errorf("effects %+v %v", ef, err)
	}

	s := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "L900", "type": "SMART.TAPOBULB", "device_on": true}, nil)
	defer s.Close()
	ds := tplink.TapoDevice{Caller: klap(t, s)}
	if err := ds.SetLightingEffect(ctx, map[string]any{"id": "demo", "name": "Demo", "enable": 1, "brightness": 70}); err != nil {
		t.Fatal(err)
	}
	if ef, err := ds.LightingEffect(ctx); err != nil || ef["name"] != "Demo" {
		t.Errorf("strip effect %v %v", ef, err)
	}
	if le, _ := s.Get("lighting_effect").(map[string]any); le["name"] != "Demo" {
		t.Errorf("device info lighting_effect %v", le)
	}
}

// A hub's sensors and buttons, typed; a batch to one child.
func TestTapoHubChildren(t *testing.T) {
	h := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "H100", "type": "SMART.TAPOHUB"}, nil)
	defer h.Close()
	h.Children = []map[string]any{
		{"device_id": "T1", "model": "T315", "category": "subg.trigger.temp-hmdt-sensor", "current_temp": 22.5, "current_humidity": 48.0, "battery_percentage": 80.0,
			"at_low_battery": false, "status": "online", "nickname": "TGl2aW5n"},
		{"device_id": "B1", "model": "S200B", "category": "subg.trigger.button", "at_low_battery": true, "status": "online"},
		{"device_id": "C1", "model": "T110", "category": "subg.trigger.contact-sensor", "open": true, "status": "online"},
	}
	h.Press("B1", "doubleClick")
	d := tplink.TapoDevice{Caller: klap(t, h)}
	kids, err := d.Children(ctx)
	if err != nil || len(kids) != 3 || *kids[0].Temp != 22.5 || kids[0].Nickname != "Living" || !kids[1].LowBattery || !*kids[2].Open {
		t.Fatalf("children %+v %v", kids, err)
	}
	cc, err := tplink.ChildComponents(ctx, d)
	if err != nil || cc["T1"]["temperature"] != 1 || cc["B1"]["trigger_log"] != 1 {
		t.Errorf("child components %v %v", cc, err)
	}
	logs, err := d.Child("B1").TriggerLogs(ctx, 5, 0)
	if err != nil || len(logs) != 1 || logs[0].Event != "doubleClick" {
		t.Errorf("logs %+v %v", logs, err)
	}
	res, err := tplink.MultiCallChild(ctx, d, "T1", []tplink.SmartRequest{{Method: "get_device_info"}, {Method: "get_temp_humidity_records"}, {Method: "no_such"}})
	if err != nil || res[0].Result["model"] != "T315" || res[1].Result["past24h_temp"] == nil || !tplink.IsUnsupported(res[2].Err) {
		t.Errorf("child batch %+v %v", res, err)
	}
	if ms := h.Methods(); ms[len(ms)-1] != "child:no_such" {
		t.Errorf("methods %v", ms)
	}
}

// The 20002 answer's encrypted part is decrypted by TapoDiscover.
func TestTapoDiscoverDecrypts(t *testing.T) {
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"device_id": "80223F", "type": "SMART.TAPOPLUG", "model": "P110", "mac": "3C-52-A1-00-00-04"}, nil)
	defer p.Close()
	p.DiscoverySecret = map[string]any{"connect_ssid": "Demo-Home", "connect_type": "wireless"}
	r, err := sim.StartResponder(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found, err := tplink.TapoDiscover(ctx, r.Addr(), 600*time.Millisecond)
	if err != nil || len(found) != 1 || found[0].Decrypted["connect_ssid"] != "Demo-Home" || found[0].Protocol() != "klap" {
		t.Errorf("found %+v %v", found, err)
	}
}
