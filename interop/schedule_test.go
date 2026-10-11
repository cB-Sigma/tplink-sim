// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

// tapoImage is the second peer: the tapo crate's Python bindings, built
// from ./tapopeer (r2: with tpap.py).
const tapoImage = "sigma-tplink-sim-tapo-interop:tapo-0.11.1-r2"

func needTapoImage(t *testing.T) {
	t.Helper()
	needDocker(t)
	if exec.Command("docker", "image", "inspect", tapoImage).Run() == nil {
		return
	}
	out, err := exec.Command("docker", "build", "-q", "-t", tapoImage, "tapopeer").CombinedOutput()
	if err != nil {
		t.Fatalf("docker build: %v\n%s", err, out)
	}
}

// runTapo runs schedule.py and collects its steps; at "ready-for-go" it
// runs between and then lets the script go on.
func runTapo(t *testing.T, between func(), args ...string) map[string]any {
	t.Helper()
	return runTapoScript(t, "schedule.py", between, args...)
}

// runTapoScript runs one of tapopeer's scripts the same way.
func runTapoScript(t *testing.T, script string, between func(), args ...string) map[string]any {
	t.Helper()
	p := startPeer(t, append([]string{"--add-host=host.docker.internal:host-gateway", tapoImage, "python", "-u", script}, args...)...)
	steps := map[string]any{}
	deadline := time.After(120 * time.Second)
	for {
		select {
		case l := <-p.lines:
			name, js, _ := strings.Cut(l, " ")
			if name == "done" {
				return steps
			}
			if name == "ready-for-go" {
				between()
				p.send("go")
				continue
			}
			var v any
			if js != "" {
				if err := json.Unmarshal([]byte(js), &v); err != nil {
					t.Fatalf("peer line %q: %v", l, err)
				}
			}
			steps[name] = v
			if strings.HasSuffix(name, "-error") {
				t.Errorf("tapo step %s: %v", name, v)
			}
		case <-deadline:
			t.Fatalf("tapo did not finish; steps %v", steps)
		}
	}
}

// The tapo crate's client (Python bindings 0.11.1) and this module's
// client on one simulated P110 over KLAP: tapo adds, lists, edits and
// removes schedules and arms and clears its timer; this client reads the
// rule tapo left and adds one that tapo then reads.
func TestTapoScheduleWrites(t *testing.T) {
	needTapoImage(t)
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"device_id": "80223P110", "model": "P110", "type": "SMART.TAPOPLUG",
		"device_on": false, "fw_ver": "1.3.0 Build 230905 Rel.152200", "hw_ver": "1.0", "mac": "3C-52-A1-00-00-01", "nickname": "UGx1Zw==",
		"region": "Europe/London", "ip": "127.0.0.1", "specs": "", "lang": "en_US", "avatar": "plug", "time_diff": 0.0, "signal_level": 3.0,
		"rssi": -40.0, "on_time": 0.0, "overheated": false, "has_set_location_info": false, "oem_id": "O1", "hw_id": "H1", "latitude": 0.0, "longitude": 0.0,
		"model_type": "", "type_name": "", "default_states": map[string]any{"type": "last_states", "state": map[string]any{}},
		"overcurrent_status": "normal", "power_protection_status": "normal", "charging_status": "normal", "auto_off_status": "off",
		"auto_off_remain_time": 0.0},
		map[string]any{"current_power": 0.0, "today_energy": 0.0, "month_energy": 0.0, "today_runtime": 0.0, "month_runtime": 0.0})
	defer p.Close()
	c := &tplink.KLAPClient{Base: "http://" + p.Addr(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: tplink.InsecureHTTP(5 * time.Second)}
	d := tplink.TapoDevice{Caller: c}
	var seen []tplink.TapoScheduleRule
	var countdowns []map[string]any
	s := runTapo(t, func() {
		var err error
		if seen, err = d.ScheduleRules(context.Background()); err != nil {
			t.Errorf("rules %v", err)
		}
		p.Do(func() { countdowns = append(countdowns, p.Rules["countdown"]...) })
		if _, err := d.AddScheduleRule(context.Background(), tplink.TapoScheduleRule{Enable: true, Start: tplink.ScheduleSunrise, Offset: 20, Days: tplink.Weekend, On: true}); err != nil {
			t.Errorf("add %v", err)
		}
	}, "host.docker.internal:"+port(t, p.Addr()), sim.DemoUser, sim.DemoPassword)
	if fe := s["fatal-error"]; fe != nil {
		t.Fatalf("tapo: %v", get(fe, "error"))
	}
	if get(s["added"], "id") == nil {
		t.Errorf("added %v", s["added"])
	}
	if rules, _ := s["rules"].([]any); len(rules) != 2 {
		t.Errorf("rules %v", s["rules"])
	}
	if s["max"] != 32.0 {
		t.Errorf("max %v", s["max"])
	}
	if s["timer-read"] == nil {
		t.Errorf("timer %v", s["timer-set"])
	}
	rules3, _ := s["rules3"].([]any)
	if len(rules3) != 1 {
		t.Fatalf("after removing one %v", s["rules3"])
	}
	// The rule tapo left, as this client reads it: the weekly rule edited
	// to 07:00, off, on Monday and Wednesday.
	if len(seen) != 1 || seen[0].ID != get(rules3[0], "id") || seen[0].Minute != 420 || seen[0].On || seen[0].Days != tplink.Monday|tplink.Wednesday ||
		seen[0].Start != tplink.ScheduleAtTime || !seen[0].Enable {
		t.Errorf("this client reads %+v; tapo %v", seen, rules3[0])
	}
	if len(countdowns) != 0 {
		t.Errorf("tapo cleared its timer; the simulator holds %v", countdowns)
	}
	// The rule this client added, as tapo reads it.
	rules4, _ := s["rules4"].([]any)
	if len(rules4) != 2 {
		t.Fatalf("tapo reads %v", s["rules4"])
	}
	t.Logf("tapo reads the rule this client added as %v", rules4[1])
	if get(rules4[1], "desired_state") != "on" || get(rules4[1], "days") != 65.0 || get(rules4[1], "time", "sunrise", "offset_minutes") != 20.0 {
		t.Errorf("tapo reads %v", rules4[1])
	}
	var left []map[string]any
	p.Do(func() { left = p.Rules["schedule"] })
	if len(left) != 0 {
		t.Errorf("tapo removed all; left %v", left)
	}
}
