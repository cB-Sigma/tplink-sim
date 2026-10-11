// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"sort"
	"time"
)

// VacuumState is a simulated robot vacuum: what its camel-case Tapo
// methods (getVacStatus, setSwitchClean, ...) read and change. Start one
// with StartVacuum, or set Tapo.Vacuum to NewVacuumState() (and the
// components of VacuumComponents) inside Do on any Tapo starter. Change
// the fields inside Do once the device is serving.
//
// The state moves as a vacuum does: setSwitchClean starts a clean
// (cleaning), setRobotPause pauses and resumes it, setSwitchCharge sends
// it home (going home; the clean is recorded) or stops the way home
// (idle). Advance lets simulated minutes pass.
type VacuumState struct {
	Status       int   // getVacStatus status: 0 idle, 1 cleaning, 2 mapping, 4 going home, 5 charging, 6 charged, 7 paused, 8 undocked, 100 error
	Errors       []int // err_status codes; empty answers [0]
	Battery      int   // percent
	Suction      int   // 1 quiet .. 5 ultra
	Cistern      int   // water: 0 off .. 3 high
	CleanNumber  int   // passes, 1 to 3
	CleanMode    int   // getCleanStatus clean_status of the current or last clean
	CleanArea    int   // of the current or last clean
	CleanMinutes int
	CleanPercent int
	// Consumables are minutes of use by getConsumablesInfo field
	// (roll_brush_time, edge_brush_time, ...).
	Consumables map[string]int
	// Records are past cleans as getCleanRecords lists them, oldest first.
	Records      []map[string]any
	TotalMinutes int
	TotalArea    int
	TotalCount   int
	CurrentMap   int64
	Maps         []VacuumMap
	Volume       int
	AutoEmpty    bool // getDustCollectionInfo auto_dust_collection
	EmptyMode    int  // dust_collection_mode: 0 smart .. 3 max
	Emptied      int  // setSwitchDustCollection requests
	MopAttached  bool
	CarpetBoost  bool
	AreaUnit     int // 0 m², 1 ft², 2 ping
	Located      int // playSelectAudio requests
	// Rooms are the room ids of the last setSwitchClean (nil: the whole
	// home).
	Rooms []int
}

// VacuumMap is a saved map of the simulated vacuum.
type VacuumMap struct {
	ID    int64
	Name  string // plain text; answered in base64
	Rooms []VacuumRoom
}

// VacuumRoom is a room of a map.
type VacuumRoom struct {
	ID    int
	Name  string // plain text; answered in base64
	Color int
}

// VacuumComponents are the components a vacuum reports in component_nego
// (a client such as python-kasa loads its vacuum modules from them).
func VacuumComponents() map[string]int {
	return map[string]int{"clean": 3, "battery": 1, "consumables": 2, "speaker": 3, "map": 2, "mop": 1, "clean_percent": 1, "dust_bucket": 1}
}

// NewVacuumState is a docked, charged vacuum with one map of four rooms,
// some history and part-worn consumables.
func NewVacuumState() *VacuumState {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC).Unix()
	v := &VacuumState{Status: 6, Battery: 100, Suction: 2, Cistern: 1, CleanNumber: 1, CleanArea: 42, CleanMinutes: 48, CleanPercent: 100,
		Consumables: map[string]int{"roll_brush_time": 1820, "edge_brush_time": 1820, "filter_time": 1820, "sensor_time": 600, "charge_contact_time": 600,
			"main_brush_lid_time": 1820, "rag_time": 0},
		CurrentMap: 1760000001, Maps: []VacuumMap{{ID: 1760000001, Name: "Home", Rooms: []VacuumRoom{{2, "Kitchen", 1}, {3, "Living room", 2}, {4, "Bedroom", 3}, {5, "Hall", 4}}}},
		Volume: 50, AutoEmpty: true, CarpetBoost: true}
	for i := 0; i < 3; i++ {
		v.Records = append(v.Records, map[string]any{"timestamp": day + int64(i)*86400, "clean_time": 48, "clean_area": 42, "dust_collection": true,
			"error": 0, "info_num": 0, "map_id": v.CurrentMap, "message": 0, "record_index": i, "start_type": 1, "task_type": 0})
	}
	v.TotalMinutes, v.TotalArea, v.TotalCount = 3*48, 3*42, 3
	return v
}

// Advance lets minutes pass: a clean goes on (area, progress, battery and
// wear), a vacuum going home docks, a docked one charges.
func (v *VacuumState) Advance(minutes int) {
	switch v.Status {
	case 1, 2:
		v.CleanMinutes += minutes
		v.CleanArea += minutes
		v.CleanPercent = min(100, v.CleanPercent+2*minutes)
		v.Battery = max(5, v.Battery-minutes/2)
		for k := range v.Consumables {
			if k != "rag_time" || v.MopAttached {
				v.Consumables[k] += minutes
			}
		}
	case 4:
		v.Status = 5
	case 5, 6:
		v.Battery = min(100, v.Battery+2*minutes)
		if v.Battery == 100 {
			v.Status = 6
		} else {
			v.Status = 5
		}
	}
}

// finish records the clean in progress.
func (v *VacuumState) finish() {
	v.Records = append(v.Records, map[string]any{"timestamp": time.Now().Unix() - int64(v.CleanMinutes)*60, "clean_time": v.CleanMinutes,
		"clean_area": v.CleanArea, "dust_collection": v.AutoEmpty, "error": 0, "info_num": 0, "map_id": v.CurrentMap, "message": 0,
		"record_index": len(v.Records), "start_type": 1, "task_type": 0})
	v.TotalMinutes += v.CleanMinutes
	v.TotalArea += v.CleanArea
	v.TotalCount++
	if v.AutoEmpty {
		v.Emptied++
	}
}

func (v *VacuumState) mapByID(id int64) *VacuumMap {
	for i := range v.Maps {
		if v.Maps[i].ID == id {
			return &v.Maps[i]
		}
	}
	return nil
}

// vacuum answers a robot vacuum request; ok is false for a method it does
// not know. The caller holds t.mu.
func (t *Tapo) vacuum(method string, params map[string]any) (map[string]any, bool) {
	v := t.Vacuum
	switch method {
	case "getVacStatus":
		errs := []any{}
		for _, e := range v.Errors {
			errs = append(errs, e)
		}
		if len(errs) == 0 {
			errs = append(errs, 0)
		}
		return smartOK(map[string]any{"status": v.Status, "err_status": errs, "errorCode_id": []any{0}, "prompt": []any{}, "promptCode_id": []any{}}), true
	case "getBatteryInfo":
		return smartOK(map[string]any{"battery_percentage": v.Battery}), true
	case "getCleanInfo":
		return smartOK(map[string]any{"clean_area": v.CleanArea, "clean_time": v.CleanMinutes, "clean_percent": v.CleanPercent}), true
	case "getCleanAttr":
		return smartOK(map[string]any{"suction": v.Suction, "cistern": v.Cistern, "clean_number": v.CleanNumber}), true
	case "getCleanStatus":
		return smartOK(map[string]any{"clean_status": v.CleanMode, "is_mapping": v.Status == 2, "is_relocating": false, "is_working": v.Status == 1 || v.Status == 2}), true
	case "getCarpetClean":
		pref := "normal"
		if v.CarpetBoost {
			pref = "boost"
		}
		return smartOK(map[string]any{"carpet_clean_prefer": pref}), true
	case "setCarpetClean":
		switch params["carpet_clean_prefer"] {
		case "boost":
			v.CarpetBoost = true
		case "normal":
			v.CarpetBoost = false
		default:
			return smartErr(-1008), true
		}
		return smartOK(nil), true
	case "getAreaUnit":
		return smartOK(map[string]any{"area_unit": v.AreaUnit}), true
	case "getMopState":
		return smartOK(map[string]any{"mop_state": v.MopAttached}), true
	case "getVolume":
		return smartOK(map[string]any{"volume": v.Volume}), true
	case "setVolume":
		vol, ok := params["volume"].(float64)
		if !ok || vol < 0 || vol > 100 {
			return smartErr(-1008), true
		}
		v.Volume = int(vol)
		return smartOK(nil), true
	case "playSelectAudio":
		v.Located++
		return smartOK(nil), true
	case "getAutoDustCollection":
		auto := 0
		if v.AutoEmpty {
			auto = 1
		}
		return smartOK(map[string]any{"auto_dust_collection": auto}), true
	case "getDustCollectionInfo":
		return smartOK(map[string]any{"auto_dust_collection": v.AutoEmpty, "dust_collection_mode": v.EmptyMode}), true
	case "setDustCollectionInfo":
		if a, ok := params["auto_dust_collection"].(bool); ok {
			v.AutoEmpty = a
		}
		if m, ok := params["dust_collection_mode"].(float64); ok {
			if m < 0 || m > 3 {
				return smartErr(-1008), true
			}
			v.EmptyMode = int(m)
		}
		return smartOK(nil), true
	case "setSwitchDustCollection":
		if params["switch_dust_collection"] == true {
			v.Emptied++
		}
		return smartOK(nil), true
	case "getConsumablesInfo":
		r := map[string]any{}
		for k, n := range v.Consumables {
			r[k] = n
		}
		return smartOK(r), true
	case "resetConsumablesTime":
		list, _ := params["reset_list"].([]any)
		for _, x := range list {
			name, _ := x.(string)
			if _, ok := v.Consumables[name+"_time"]; !ok {
				return smartErr(-1008), true
			}
		}
		for _, x := range list {
			v.Consumables[x.(string)+"_time"] = 0
		}
		return smartOK(nil), true
	case "getCleanRecords":
		list := []any{}
		for _, r := range v.Records {
			list = append(list, copyMap(r))
		}
		r := map[string]any{"total_time": v.TotalMinutes, "total_area": v.TotalArea, "total_number": v.TotalCount, "record_list": list, "record_list_num": len(list)}
		if n := len(v.Records); n > 0 {
			last := v.Records[n-1]
			dust := 0
			if last["dust_collection"] == true {
				dust = 1
			}
			r["lastest_day_record"] = []any{last["timestamp"], last["clean_time"], last["clean_area"], dust}
		}
		return smartOK(r), true
	case "getMapInfo":
		list := []any{}
		for _, m := range v.Maps {
			list = append(list, map[string]any{"map_id": m.ID, "map_name": b64(m.Name), "is_saved": true, "map_locked": 0, "auto_area_flag": true,
				"global_cleaned": -1, "rotate_angle": 0, "update_time": time.Now().Unix()})
		}
		return smartOK(map[string]any{"current_map_id": v.CurrentMap, "map_list": list, "map_num": len(list), "auto_change_map": false, "version": "LDS"}), true
	case "getMapData":
		m := v.mapByID(int64(num(params["map_id"])))
		if m == nil {
			return smartErr(-1008), true
		}
		areas := []any{}
		for _, r := range m.Rooms {
			areas = append(areas, map[string]any{"id": r.ID, "name": b64(r.Name), "type": "room", "color": r.Color, "suction": v.Suction,
				"cistern": v.Cistern, "clean_number": v.CleanNumber, "floor_texture": -1})
		}
		return smartOK(map[string]any{"map_id": m.ID, "map_name": b64(m.Name), "area_list": areas, "map_locked": 0, "auto_area_flag": true,
			"resolution": 50, "resolution_unit": "mm", "width": 0, "height": 0, "map_data": "", "version": "LDS"}), true
	case "setCleanAttr":
		set := func(key string, lo, hi int, dst *int) bool {
			x, ok := params[key].(float64)
			if !ok {
				return true
			}
			if int(x) < lo || int(x) > hi {
				return false
			}
			*dst = int(x)
			return true
		}
		s, c, n := v.Suction, v.Cistern, v.CleanNumber
		if !set("suction", 1, 5, &s) || !set("cistern", 0, 3, &c) || !set("clean_number", 1, 3, &n) {
			return smartErr(-1008), true
		}
		v.Suction, v.Cistern, v.CleanNumber = s, c, n
		return smartOK(nil), true
	case "setSwitchClean":
		if params["clean_on"] != true {
			if v.Status == 1 || v.Status == 7 {
				v.finish()
			}
			v.Status = 0
			return smartOK(nil), true
		}
		v.Rooms = nil
		if list, ok := params["room_list"].([]any); ok {
			id, ok := params["map_id"].(float64)
			m := v.mapByID(int64(id))
			if !ok || m == nil || len(list) == 0 {
				return smartErr(-1008), true
			}
			for _, x := range list {
				known := false
				for _, r := range m.Rooms {
					known = known || float64(r.ID) == x
				}
				if !known {
					return smartErr(-1008), true
				}
				v.Rooms = append(v.Rooms, int(num(x)))
			}
			sort.Ints(v.Rooms)
		}
		v.CleanMode = int(num(params["clean_mode"]))
		v.Status, v.CleanArea, v.CleanMinutes, v.CleanPercent = 1, 0, 0, 0
		return smartOK(nil), true
	case "setRobotPause":
		switch {
		case params["pause"] == true && (v.Status == 1 || v.Status == 2 || v.Status == 4):
			v.Status = 7
		case params["pause"] == false && v.Status == 7:
			v.Status = 1
		}
		return smartOK(nil), true
	case "setSwitchCharge":
		if params["switch_charge"] == true {
			if v.Status == 1 || v.Status == 7 {
				v.finish()
			}
			if v.Status != 5 && v.Status != 6 {
				v.Status = 4
			}
		} else if v.Status == 4 {
			v.Status = 0
		}
		return smartOK(nil), true
	}
	return nil, false
}

// StartVacuum starts a robot vacuum on the plain HTTPS transport
// (StartTapoSSL) with NewVacuumState and VacuumComponents.
func StartVacuum(username, password string, info map[string]any) *Tapo {
	t := StartTapoSSL(username, password, info)
	t.Do(func() {
		t.Vacuum = NewVacuumState()
		if t.Components == nil {
			t.Components = map[string]int{}
		}
		for k, n := range VacuumComponents() {
			t.Components[k] = n
		}
	})
	return t
}
