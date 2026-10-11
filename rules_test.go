// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"errors"
	"testing"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

// Tapo schedules and countdowns changed and read back.
func TestTapoRuleWrites(t *testing.T) {
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "P110", "type": "SMART.TAPOPLUG", "device_on": false}, nil)
	defer p.Close()
	d := tplink.TapoDevice{Caller: klap(t, p)}

	morning := tplink.TapoScheduleRule{Enable: true, Start: tplink.ScheduleAtTime, Minute: 6*60 + 30, Days: tplink.Weekdays, On: true}
	id, err := d.AddScheduleRule(ctx, morning)
	if err != nil || id == "" {
		t.Fatalf("add %q %v", id, err)
	}
	dusk := tplink.TapoScheduleRule{Enable: true, Start: tplink.ScheduleSunset, Offset: -15, On: true}
	id2, err := d.AddScheduleRule(ctx, dusk)
	if err != nil || id2 == id {
		t.Fatalf("add sunset %q %v", id2, err)
	}
	var se *tplink.SmartError
	if _, err := d.AddScheduleRule(ctx, morning); !errors.As(err, &se) || se.Code != -1803 {
		t.Errorf("a rule at the same time: %v", err)
	}
	if _, err := d.AddScheduleRule(ctx, tplink.TapoScheduleRule{Minute: 1440}); err == nil {
		t.Error("a minute past the day was sent")
	}
	rules, err := d.ScheduleRules(ctx)
	if err != nil || len(rules) != 2 {
		t.Fatalf("rules %+v %v", rules, err)
	}
	got := rules[0]
	if got.ID != id || got.Minute != 390 || got.Days != tplink.Weekdays || !got.On || !got.Enable || got.Start != tplink.ScheduleAtTime {
		t.Errorf("first rule %+v", got)
	}
	if r := rules[1]; r.Start != tplink.ScheduleSunset || r.Offset != -15 || r.Days != 0 {
		t.Errorf("second rule %+v", r)
	}

	// Edit the read rule: the fields the client does not model go back.
	got.Minute, got.On, got.Days = 7*60, false, tplink.EveryDay
	if err := d.EditScheduleRule(ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := d.EditScheduleRule(ctx, tplink.TapoScheduleRule{Minute: 1}); err == nil {
		t.Error("an edit without an id was sent")
	}
	if err := d.EditScheduleRule(ctx, tplink.TapoScheduleRule{ID: "S99", Minute: 1}); !errors.As(err, &se) || se.Code != -1805 {
		t.Errorf("an unknown id: %v", err)
	}
	rules, _ = d.ScheduleRules(ctx)
	if rules[0].Minute != 420 || rules[0].On || rules[0].Days != tplink.EveryDay || rules[0].ID != id {
		t.Errorf("edited %+v", rules[0])
	}
	var raw map[string]any
	p.Do(func() { raw = p.Rules["schedule"][0] })
	if raw["mode"] != "repeat" || raw["week_day"] != 127.0 || raw["e_action"] != "none" || raw["year"] != 1970.0 {
		t.Errorf("stored %v", raw)
	}

	if err := d.RemoveScheduleRules(ctx, id); err != nil {
		t.Fatal(err)
	}
	if rules, _ := d.ScheduleRules(ctx); len(rules) != 1 || rules[0].ID != id2 {
		t.Errorf("after removing one %+v", rules)
	}
	if err := d.RemoveScheduleRules(ctx); err != nil {
		t.Fatal(err)
	}
	if rules, _ := d.ScheduleRules(ctx); len(rules) != 0 {
		t.Errorf("after removing all %+v", rules)
	}

	// Countdown: one at a time.
	cid, err := d.AddCountdown(ctx, 600, true)
	if err != nil || cid == "" {
		t.Fatalf("countdown %q %v", cid, err)
	}
	if _, err := d.AddCountdown(ctx, 60, false); !errors.As(err, &se) || se.Code != -1902 {
		t.Errorf("a second countdown: %v", err)
	}
	if _, err := d.AddCountdown(ctx, 0, false); err == nil {
		t.Error("a zero delay was sent")
	}
	cds, err := d.CountdownRules(ctx)
	if err != nil || len(cds) != 1 || cds[0].Delay != 600 || !cds[0].On || !cds[0].Enable || cds[0].ID != cid {
		t.Errorf("countdowns %+v %v", cds, err)
	}
	if _, err := d.ReplaceCountdown(ctx, 120, false); err != nil {
		t.Fatal(err)
	}
	if cds, _ := d.CountdownRules(ctx); len(cds) != 1 || cds[0].Delay != 120 || cds[0].On {
		t.Errorf("replaced %+v", cds)
	}
	if err := d.RemoveCountdowns(ctx); err != nil {
		t.Fatal(err)
	}
	if cds, _ := d.CountdownRules(ctx); len(cds) != 0 {
		t.Errorf("removed %+v", cds)
	}
}

// Rules recorded by older firmware give the state as s_action.
func TestParseTapoScheduleRule(t *testing.T) {
	r := tplink.ParseTapoScheduleRule(map[string]any{"id": "S3", "enable": false, "s_type": "normal", "s_min": 90.0, "mode": "once",
		"week_day": 0.0, "s_action": "off"})
	if r.ID != "S3" || r.Minute != 90 || r.On || r.Enable || r.Days != 0 {
		t.Errorf("%+v", r)
	}
	r = tplink.ParseTapoScheduleRule(map[string]any{"id": "S4", "s_type": "sunrise", "time_offset": 30.0, "mode": "repeat", "week_day": 62.0,
		"desired_states": map[string]any{"on": true}, "s_action": "off"})
	if r.Start != tplink.ScheduleSunrise || r.Offset != 30 || r.Days != tplink.Weekdays || !r.On {
		t.Errorf("%+v", r)
	}
}
