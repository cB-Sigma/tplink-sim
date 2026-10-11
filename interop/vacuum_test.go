// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"testing"

	"github.com/cB-Sigma/tplink-sim/sim"
)

// python-kasa's Device API against the simulated vacuum on the plain
// HTTPS transport: its vacuum modules read the state, and start, pause
// and return home move the simulator.
func TestVacuumModules(t *testing.T) {
	needDocker(t)
	info := tapoInfo("RV30 Max", "7C-F1-7E-00-00-01")
	info["type"], info["nickname"] = "SMART.TAPOROBOVAC", "VmFj"
	delete(info, "device_on")
	v := sim.StartVacuum(sim.DemoUser, sim.DemoPassword, info)
	defer v.Close()
	s := run(t, "vacuum", "host.docker.internal", port(t, v.Addr()), sim.DemoUser, sim.DemoPassword)
	if get(s["vacuum"], "type") != "DeviceType.Vacuum" || get(s["vacuum"], "status") != "Charged" || get(s["vacuum"], "battery") != 100.0 ||
		get(s["vacuum"], "fan") != "Standard" || get(s["vacuum"], "count") != 1.0 || get(s["vacuum"], "carpet") != true || get(s["vacuum"], "area") != 42.0 {
		t.Errorf("vacuum %v", s["vacuum"])
	}
	if get(s["consumables"], "main_brush") != 1820.0 || get(s["consumables"], "sensor") != 600.0 {
		t.Errorf("consumables %v", s["consumables"])
	}
	if get(s["records"], "count") != 3.0 || get(s["records"], "area") != 126.0 || get(s["records"], "last_area") != 42.0 {
		t.Errorf("records %v", s["records"])
	}
	if get(s["speaker"], "volume") != 50.0 || get(s["mop"], "attached") != false || get(s["dustbin"], "auto") != true || get(s["dustbin"], "mode") != "Smart" {
		t.Errorf("speaker %v mop %v dustbin %v", s["speaker"], s["mop"], s["dustbin"])
	}
	for step, want := range map[string]string{"start-state": "Cleaning", "pause-state": "Paused", "return-home-state": "GoingHome"} {
		if get(s[step], "status") != want {
			t.Errorf("%s %v", step, s[step])
		}
	}
	if get(s["device2"], "fan") != "Max" || get(s["device2"], "volume") != 30.0 || get(s["device2"], "records") != 4.0 {
		t.Errorf("device2 %v", s["device2"])
	}
	var st sim.VacuumState
	v.Do(func() { st = *v.Vacuum })
	// One emptying after the clean (auto-empty) and one asked for.
	if st.Status != 4 || st.Suction != 4 || st.Volume != 30 || st.TotalCount != 4 || st.Emptied != 2 {
		t.Errorf("simulator %+v", st)
	}
}
