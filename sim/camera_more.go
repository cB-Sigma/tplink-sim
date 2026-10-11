// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"fmt"
	"strconv"
	"time"
)

// CamClip is a simulated recording (searchDateWithVideo,
// searchVideoWithUTC).
type CamClip struct {
	Start, End time.Time
	Type       int // video_type: 1 timed, 2 motion, ...
}

// CamState is what a simulated camera does beyond its settings: the
// motor's position and saved positions, the manual alarm, reboots,
// formats and recordings. Settings themselves live in CamSections.
type CamState struct {
	// Recordings are the clips the camera reports.
	Recordings []CamClip

	pan, tilt   int
	presetNo    int
	alarm       bool
	camReboots  int
	formats     int
	calibrated  int
	singleCalls int
}

// CamPosition is the simulated motor's position (pan, tilt).
func (t *Tapo) CamPosition() (pan, tilt int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pan, t.tilt
}

// CamAlarm reports whether the manual alarm is sounding.
func (t *Tapo) CamAlarm() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alarm
}

// CamCounts are how many reboots, SD card formats, calibrations and
// single-form requests the camera took.
func (t *Tapo) CamCounts() (reboots, formats, calibrations, single int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.camReboots, t.formats, t.calibrated, t.singleCalls
}

func atoiAny(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

// presets returns the preset section (id and name lists), creating it.
func (t *Tapo) presets() map[string]any {
	sec := t.camSection("preset")
	pr, _ := sec["preset"].(map[string]any)
	if pr == nil {
		pr = map[string]any{"id": []any{}, "name": []any{}, "position_pan": []any{}, "position_tilt": []any{}, "read_only": []any{}}
		sec["preset"] = pr
	}
	return pr
}

// camMore answers the camera requests that act rather than read or write a
// setting; ok is false for others. The caller holds t.mu.
func (t *Tapo) camMore(method string, p map[string]any) (map[string]any, int, bool) {
	switch method {
	case "motorMove":
		mv, _ := p["motor"].(map[string]any)
		m, _ := mv["move"].(map[string]any)
		if m == nil {
			return nil, -40209, true
		}
		t.pan += atoiAny(m["x_coord"])
		t.tilt += atoiAny(m["y_coord"])
		return map[string]any{}, 0, true
	case "relativeMove":
		mv, _ := p["motor"].(map[string]any)
		ms, _ := mv["movestep"].(map[string]any)
		if ms == nil {
			return nil, -40209, true
		}
		switch atoiAny(ms["direction"]) {
		case 0:
			t.pan += 10
		case 90:
			t.tilt += 10
		case 180:
			t.pan -= 10
		case 270:
			t.tilt -= 10
		}
		return map[string]any{}, 0, true
	case "manualCalibrate":
		t.pan, t.tilt = 0, 0
		t.calibrated++
		return map[string]any{}, 0, true
	case "addMotorPostion":
		pp, _ := p["preset"].(map[string]any)
		sp, _ := pp["set_preset"].(map[string]any)
		name, _ := sp["name"].(string)
		if name == "" {
			return nil, -40209, true
		}
		pr := t.presets()
		t.presetNo++
		for k, v := range map[string]any{"id": strconv.Itoa(t.presetNo), "name": name, "position_pan": fmt.Sprint(t.pan), "position_tilt": fmt.Sprint(t.tilt),
			"read_only": "0"} {
			l, _ := pr[k].([]any)
			pr[k] = append(l, v)
		}
		return map[string]any{}, 0, true
	case "motorMoveToPreset":
		pp, _ := p["preset"].(map[string]any)
		gp, _ := pp["goto_preset"].(map[string]any)
		pr := t.presets()
		ids, _ := pr["id"].([]any)
		for i, id := range ids {
			if id == gp["id"] {
				pans, _ := pr["position_pan"].([]any)
				tilts, _ := pr["position_tilt"].([]any)
				if i < len(pans) && i < len(tilts) {
					t.pan, t.tilt = atoiAny(pans[i]), atoiAny(tilts[i])
				}
				return map[string]any{}, 0, true
			}
		}
		return nil, -40209, true
	case "deletePreset":
		pp, _ := p["preset"].(map[string]any)
		rp, _ := pp["remove_preset"].(map[string]any)
		drop := map[any]bool{}
		list, _ := rp["id"].([]any)
		for _, id := range list {
			drop[id] = true
		}
		pr := t.presets()
		ids, _ := pr["id"].([]any)
		keep := map[string][]any{}
		for i, id := range ids {
			if drop[id] {
				continue
			}
			for _, k := range []string{"id", "name", "position_pan", "position_tilt", "read_only"} {
				l, _ := pr[k].([]any)
				if i < len(l) {
					keep[k] = append(keep[k], l[i])
				}
			}
		}
		for _, k := range []string{"id", "name", "position_pan", "position_tilt", "read_only"} {
			pr[k] = append([]any{}, keep[k]...)
		}
		return map[string]any{}, 0, true
	case "rebootDevice":
		t.camReboots++
		return map[string]any{}, 0, true
	case "formatSdCard":
		t.formats++
		return map[string]any{}, 0, true
	case "getSirenStatus", "getSirenConfig", "getSirenTypeList":
		// Answered without the module level, as recorded devices do.
		if _, ok := t.CamSections["siren"]; !ok {
			return nil, -40106, true
		}
		sec := t.camSection("siren")
		keys := map[string][]string{"getSirenStatus": {"status", "time_left"}, "getSirenConfig": {"siren_type", "volume", "duration"},
			"getSirenTypeList": {"siren_type_list"}}[method]
		out := map[string]any{}
		for _, k := range keys {
			if v, ok := sec[k]; ok {
				out[k] = v
			}
		}
		return out, 0, true
	case "setSirenConfig":
		s, _ := p["siren"].(map[string]any)
		mergeInto(t.camSection("siren"), s)
		return map[string]any{}, 0, true
	case "setSirenStatus":
		s, _ := p["siren"].(map[string]any)
		sec := t.camSection("siren")
		sec["status"] = s["status"]
		return map[string]any{}, 0, true
	case "scanApList":
		return map[string]any{"onboarding": map[string]any{"scan": map[string]any{"ap_list": []any{
			map[string]any{"ssid": "Demo-Home", "bssid": "00:00:00:00:00:01", "auth": 3, "encryption": 2, "rssi": 3},
		}, "wpa3_supported": "false", "publicKey": ""}}}, 0, true
	case "searchDateWithVideo":
		pb, _ := p["playback"].(map[string]any)
		q, _ := pb["search_year_utility"].(map[string]any)
		from, to := str(q["start_date"]), str(q["end_date"])
		seen := map[string]bool{}
		results := []any{}
		for _, c := range t.Recordings {
			d := c.Start.UTC().Format("20060102")
			if d < from || d > to || seen[d] {
				continue
			}
			seen[d] = true
			results = append(results, map[string]any{fmt.Sprintf("search_results_%d", len(results)+1): map[string]any{"date": d}})
		}
		return map[string]any{"playback": map[string]any{"search_results": results}}, 0, true
	case "searchVideoWithUTC":
		pb, _ := p["playback"].(map[string]any)
		q, _ := pb["search_video_with_utc"].(map[string]any)
		from, to := int64(num(q["start_time"])), int64(num(q["end_time"]))
		start, end := int(num(q["start_index"])), int(num(q["end_index"]))
		results := []any{}
		n := 0
		for _, c := range t.Recordings {
			if c.Start.Unix() < from || c.Start.Unix() > to {
				continue
			}
			if n >= start && n <= end {
				results = append(results, map[string]any{fmt.Sprintf("search_video_results_%d", len(results)+1): map[string]any{
					"startTime": c.Start.Unix(), "endTime": c.End.Unix(), "video_type": c.Type}})
			}
			n++
		}
		return map[string]any{"playback": map[string]any{"search_video_results": results}}, 0, true
	}
	return nil, 0, false
}

// camDo answers the single "do" form for the motor and the manual alarm;
// ok is false for other modules. The caller holds t.mu.
func (t *Tapo) camDo(module string, body map[string]any) (map[string]any, bool) {
	t.singleCalls++
	switch module {
	case "motor":
		if _, code, _ := t.camMore("motorMove", map[string]any{"motor": body}); code != 0 {
			return map[string]any{"error_code": code}, true
		}
		return map[string]any{"error_code": 0}, true
	case "msg_alarm":
		ma, _ := body["manual_msg_alarm"].(map[string]any)
		switch ma["action"] {
		case "start":
			t.alarm = true
		case "stop":
			t.alarm = false
		default:
			return map[string]any{"error_code": -40209}, true
		}
		return map[string]any{"error_code": 0}, true
	}
	return nil, false
}
