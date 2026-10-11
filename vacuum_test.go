// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

func vacuumInfo() map[string]any {
	return map[string]any{"model": "RV30 Max", "type": "SMART.TAPOROBOVAC", "nickname": "VmFj", "device_id": "80223V"}
}

// Every TapoVacuum helper against the simulated vacuum on the plain HTTPS
// transport: reading the state, a clean of two rooms paused, resumed and
// sent home, settings changed and read back.
func TestTapoVacuumPlainHTTPS(t *testing.T) {
	d := sim.StartVacuum(sim.DemoUser, sim.DemoPassword, vacuumInfo())
	defer d.Close()
	v := tplink.TapoVacuum{Caller: &tplink.SSLClient{Base: "https://" + d.Addr(), Username: sim.DemoUser, Password: sim.DemoPassword, HTTP: tplink.InsecureHTTP(5 * time.Second)}}
	vac := func() (s sim.VacuumState) { d.Do(func() { s = *d.Vacuum }); return }

	st, err := v.Status(ctx)
	if err != nil || st.Status != tplink.VacuumCharged || st.Battery != 100 || st.Suction != tplink.SuctionStandard || st.Water != tplink.WaterLow ||
		st.CleanCount != 1 || st.CleanArea != 42 || st.CleanMinutes != 48 || st.CleanPercent != 100 || st.Faulted() || st.Working || len(st.Raw) != 5 {
		t.Fatalf("status %+v %v", st, err)
	}
	maps, err := v.Maps(ctx)
	if err != nil || maps.Current != 1760000001 || len(maps.Maps) != 1 || maps.Maps[0].Name != "Home" {
		t.Fatalf("maps %+v %v", maps, err)
	}
	rooms, err := v.Rooms(ctx, maps.Current)
	if err != nil || len(rooms) != 4 || rooms[0].Name != "Kitchen" || rooms[3] != (tplink.VacuumRoom{ID: 5, Name: "Hall", Color: 4, Suction: 2, Water: 1, CleanCount: 1}) {
		t.Fatalf("rooms %+v %v", rooms, err)
	}
	if _, err := v.Rooms(ctx, 99); err == nil {
		t.Error("an unknown map answered")
	}

	// A clean of two rooms, paused, resumed, worked for 20 minutes and sent
	// home; the clean is recorded.
	if err := v.CleanRooms(ctx, maps.Current, []int{3, 2}); err != nil {
		t.Fatal(err)
	}
	if s := vac(); s.Status != 1 || !reflect.DeepEqual(s.Rooms, []int{2, 3}) || s.CleanMode != 3 {
		t.Errorf("after CleanRooms %+v", s)
	}
	if err := v.CleanRooms(ctx, maps.Current, []int{42}); err == nil {
		t.Errorf("unknown room %v", err)
	}
	if err := v.Pause(ctx); err != nil || vac().Status != 7 {
		t.Errorf("pause %v %d", err, vac().Status)
	}
	if err := v.Resume(ctx); err != nil || vac().Status != 1 {
		t.Errorf("resume %v %d", err, vac().Status)
	}
	d.Do(func() { d.Vacuum.Advance(20) })
	st, err = v.Status(ctx)
	if err != nil || st.Status != tplink.VacuumCleaning || !st.Working || st.Mode != tplink.VacuumModeRooms || st.CleanMinutes != 20 || st.CleanArea != 20 || st.Battery != 90 {
		t.Errorf("cleaning %+v %v", st, err)
	}
	if err := v.ReturnHome(ctx); err != nil || vac().Status != 4 {
		t.Errorf("return home %v %d", err, vac().Status)
	}
	if err := v.StopReturn(ctx); err != nil || vac().Status != 0 {
		t.Errorf("stop return %v %d", err, vac().Status)
	}
	if err := v.ReturnHome(ctx); err != nil {
		t.Fatal(err)
	}
	d.Do(func() { d.Vacuum.Advance(1) })
	if s := vac(); s.Status != 5 {
		t.Errorf("docked %d", s.Status)
	}
	rec, err := v.CleanRecords(ctx)
	if err != nil || rec.TotalCount != 4 || rec.TotalMinutes != 3*48+20 || len(rec.Records) != 4 || rec.Last == nil || rec.Last.Minutes != 20 || rec.Last.Area != 20 ||
		!rec.Last.DustCollection || rec.Records[3].MapID != 1760000001 {
		t.Errorf("records %+v %v", rec, err)
	}
	if err := v.Start(ctx); err != nil || vac().Status != 1 || vac().Rooms != nil || vac().CleanMode != 0 {
		t.Errorf("start %v %+v", err, vac())
	}

	// Settings.
	if err := v.SetSuction(ctx, tplink.SuctionUltra); err != nil {
		t.Fatal(err)
	}
	if err := v.SetWaterLevel(ctx, tplink.WaterHigh); err != nil {
		t.Fatal(err)
	}
	if err := v.SetCleanCount(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := v.SetCleanCount(ctx, 9); err == nil {
		t.Error("nine passes accepted")
	}
	if st, _ := v.Status(ctx); st.Suction != tplink.SuctionUltra || st.Water != tplink.WaterHigh || st.CleanCount != 2 {
		t.Errorf("attributes %+v", st)
	}
	c, err := v.Consumables(ctx)
	if err != nil || c.MainBrush != 1840 || c.Sensor != 620 || c.Mop != 0 || len(c.Used) != 7 {
		t.Errorf("consumables %+v %v", c, err)
	}
	if err := v.ResetConsumable(ctx, tplink.ConsumableFilter); err != nil {
		t.Fatal(err)
	}
	if err := v.ResetConsumable(ctx, "nothing"); err == nil {
		t.Error("unknown consumable reset")
	}
	if c, _ := v.Consumables(ctx); c.Filter != 0 || c.MainBrush != 1840 {
		t.Errorf("after reset %+v", c)
	}
	if mop, err := v.MopAttached(ctx); err != nil || mop {
		t.Errorf("mop %v %v", mop, err)
	}
	d.Do(func() { d.Vacuum.MopAttached = true })
	if mop, _ := v.MopAttached(ctx); !mop {
		t.Error("mop not attached")
	}
	if dc, err := v.DustCollection(ctx); err != nil || !dc.Auto || dc.Mode != tplink.DustSmart {
		t.Errorf("dust %+v %v", dc, err)
	}
	if err := v.SetDustCollection(ctx, tplink.VacuumDustCollection{Auto: false, Mode: tplink.DustMax}); err != nil {
		t.Fatal(err)
	}
	if dc, _ := v.DustCollection(ctx); dc.Auto || dc.Mode != tplink.DustMax {
		t.Errorf("dust after set %+v", dc)
	}
	before := vac().Emptied
	if err := v.EmptyDustbin(ctx); err != nil || vac().Emptied != before+1 {
		t.Errorf("empty %v %d", err, vac().Emptied)
	}
	if vol, err := v.Volume(ctx); err != nil || vol != 50 {
		t.Errorf("volume %v %v", vol, err)
	}
	if err := v.SetVolume(ctx, 80); err != nil {
		t.Fatal(err)
	}
	if vol, _ := v.Volume(ctx); vol != 80 {
		t.Errorf("volume after set %d", vol)
	}
	if err := v.Locate(ctx); err != nil || vac().Located != 1 {
		t.Errorf("locate %v %d", err, vac().Located)
	}
	if b, err := v.CarpetBoost(ctx); err != nil || !b {
		t.Errorf("carpet %v %v", b, err)
	}
	if err := v.SetCarpetBoost(ctx, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := v.CarpetBoost(ctx); b {
		t.Error("carpet boost still on")
	}
	if u, err := v.AreaUnit(ctx); err != nil || u != tplink.AreaSquareMetres {
		t.Errorf("area unit %v %v", u, err)
	}
	d.Do(func() { d.Vacuum.Errors = []int{22} })
	if st, _ := v.Status(ctx); !st.Faulted() || !reflect.DeepEqual(st.Errors, []int{22}) {
		t.Errorf("fault %+v", st)
	}
	// The device itself still answers its device information.
	if info, err := (tplink.TapoDevice{Caller: v.Caller}).Info(ctx); err != nil || info.Model != "RV30 Max" {
		t.Errorf("info %+v %v", info, err)
	}
}

// The same vacuum state over KLAP (newer firmware).
func TestTapoVacuumKLAP(t *testing.T) {
	d := sim.StartTapo(sim.DemoUser, sim.DemoPassword, vacuumInfo(), nil)
	defer d.Close()
	d.Do(func() { d.Vacuum = sim.NewVacuumState(); d.Components = sim.VacuumComponents() })
	v := tplink.TapoVacuum{Caller: &tplink.KLAPClient{Base: "http://" + d.Addr(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: tplink.InsecureHTTP(5 * time.Second)}}
	comp, err := tplink.Components(ctx, v.Caller)
	if err != nil || comp["clean"] != 3 || comp["dust_bucket"] != 1 {
		t.Errorf("components %v %v", comp, err)
	}
	if err := v.Start(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := v.Status(ctx)
	if err != nil || st.Status != tplink.VacuumCleaning || st.Battery != 100 {
		t.Errorf("status %+v %v", st, err)
	}
	if err := v.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if err := v.ReturnHome(ctx); err != nil {
		t.Fatal(err)
	}
	if rec, err := v.CleanRecords(ctx); err != nil || rec.TotalCount != 4 {
		t.Errorf("records %+v %v", rec, err)
	}
}
