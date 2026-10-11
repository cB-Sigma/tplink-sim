// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import "strconv"

// Error codes of the rule requests, as devices answer them.
const (
	errScheduleFull     = -1802
	errScheduleConflict = -1803
	errScheduleIndex    = -1805
	errCountdownBusy    = -1902
	errParams           = -1008
	scheduleMaxRules    = 32
)

// ruleWrite answers the requests that change Tapo rule lists:
// add_schedule_rule, edit_schedule_rule, remove_schedule_rules,
// add_countdown_rule and remove_countdown_rules; ok is false for other
// methods. Rules are kept in Rules by kind. The caller holds t.mu.
func (t *Tapo) ruleWrite(method string, params map[string]any) (map[string]any, bool) {
	if t.Rules == nil {
		t.Rules = map[string][]map[string]any{}
	}
	switch method {
	case "add_schedule_rule":
		rules := t.Rules["schedule"]
		if len(rules) >= scheduleMaxRules {
			return smartErr(errScheduleFull), true
		}
		if !validSchedule(params) {
			return smartErr(errParams), true
		}
		for _, r := range rules {
			if sameStart(r, params) {
				return smartErr(errScheduleConflict), true
			}
		}
		t.ruleID++
		r := copyMap(params)
		r["id"] = "S" + strconv.Itoa(t.ruleID)
		t.Rules["schedule"] = append(rules, r)
		return smartOK(map[string]any{"id": r["id"]}), true
	case "edit_schedule_rule":
		if !validSchedule(params) {
			return smartErr(errParams), true
		}
		rules := t.Rules["schedule"]
		for i, r := range rules {
			if r["id"] == params["id"] {
				for j, o := range rules {
					if j != i && sameStart(o, params) {
						return smartErr(errScheduleConflict), true
					}
				}
				rules[i] = copyMap(params)
				return smartOK(nil), true
			}
		}
		return smartErr(errScheduleIndex), true
	case "remove_schedule_rules":
		if params["remove_all"] == true {
			t.Rules["schedule"] = nil
			return smartOK(nil), true
		}
		list, _ := params["rule_list"].([]any)
		drop := map[string]bool{}
		for _, x := range list {
			m, _ := x.(map[string]any)
			drop[str(m["id"])] = true
		}
		kept := []map[string]any{}
		for _, r := range t.Rules["schedule"] {
			if !drop[str(r["id"])] {
				kept = append(kept, r)
			}
		}
		t.Rules["schedule"] = kept
		return smartOK(nil), true
	case "add_countdown_rule":
		if len(t.Rules["countdown"]) > 0 {
			return smartErr(errCountdownBusy), true
		}
		delay := num(params["delay"])
		ds, _ := params["desired_states"].(map[string]any)
		if delay <= 0 || ds == nil {
			return smartErr(errParams), true
		}
		t.ruleID++
		r := map[string]any{"id": "C" + strconv.Itoa(t.ruleID), "enable": params["enable"] == true, "delay": delay, "remain": delay,
			"desired_states": copyMap(ds)}
		t.Rules["countdown"] = []map[string]any{r}
		return smartOK(map[string]any{"id": r["id"]}), true
	case "remove_countdown_rules":
		t.Rules["countdown"] = nil
		return smartOK(nil), true
	}
	return nil, false
}

// validSchedule checks the fields a device needs in a schedule rule.
func validSchedule(p map[string]any) bool {
	for _, k := range []string{"enable", "s_type", "s_min", "time_offset", "mode", "week_day", "desired_states", "year", "month", "day", "e_type", "e_action", "e_min"} {
		if _, ok := p[k]; !ok {
			return false
		}
	}
	switch str(p["s_type"]) {
	case "normal":
		m := num(p["s_min"])
		return m >= 0 && m < 1440
	case "sunrise", "sunset":
		return true
	}
	return false
}

// sameStart reports two rules starting at the same time on the same days.
func sameStart(a, b map[string]any) bool {
	return str(a["s_type"]) == str(b["s_type"]) && num(a["s_min"]) == num(b["s_min"]) && num(a["time_offset"]) == num(b["time_offset"]) &&
		str(a["mode"]) == str(b["mode"]) && num(a["week_day"]) == num(b["week_day"])
}
