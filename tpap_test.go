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

func tpapPlugInfo() map[string]any {
	return map[string]any{"device_id": "80223T", "model": "P110M", "type": "SMART.TAPOPLUG", "fw_ver": "1.4.0 Build 250101",
		"mac": "3C-52-A1-00-00-20", "nickname": "UGx1Zw==", "device_on": false}
}

// A plug in TPAP mode: discover, the login, requests, typed helpers, a
// reboot, a wrong password, KLAP refused, and the lock after failed logins.
//
// Covers: SIG-IMP-179
func TestTPAPPlug(t *testing.T) {
	p := sim.StartTapoTPAP(sim.DemoUser, sim.DemoPassword, tpapPlugInfo(), map[string]any{"current_power": 12.0, "today_energy": 30.0, "month_energy": 900.0})
	defer p.Close()
	p.Do(func() { p.TPAP.LockAfter = 3 })
	hc := &http.Client{Timeout: 5 * time.Second}

	info, err := tplink.TPAPDiscover(ctx, hc, "http://"+p.Addr())
	if err != nil || info == nil {
		t.Fatalf("discover %v %v", info, err)
	}
	if pt, _ := info.PasscodeType(); pt != "userpw" || info.TLS != 0 {
		t.Errorf("info %+v", info)
	}
	c := &tplink.TPAPClient{Base: info.BaseURL(p.Addr()), Password: sim.DemoPassword, HTTP: hc}
	got, err := c.Call(ctx, "get_device_info", nil)
	if err != nil || got["model"] != "P110M" {
		t.Fatalf("get_device_info %v %v", got, err)
	}
	d := tplink.TapoDevice{Caller: c}
	if err := d.SetOn(ctx, true); err != nil || p.Get("device_on") != true {
		t.Fatalf("switch on: %v, device_on %v", err, p.Get("device_on"))
	}
	if u, err := c.Call(ctx, "get_energy_usage", nil); err != nil || u["current_power"] != 12.0 {
		t.Errorf("energy %v %v", u, err)
	}
	res, err := tplink.MultiCall(ctx, c, []tplink.SmartRequest{{Method: "get_device_info"}, {Method: "get_energy_usage"}}, 0)
	if err != nil || len(res) != 2 || res[0].Err != nil {
		t.Errorf("multipleRequest %v %v", res, err)
	}
	if n := p.TPAPLogins(); n != 1 {
		t.Errorf("%d logins for one session", n)
	}
	p.ForgetTPAPSessions()
	if _, err := c.Call(ctx, "get_device_info", nil); err != nil || p.TPAPLogins() != 2 {
		t.Errorf("after a reboot: %v, logins %d", err, p.TPAPLogins())
	}

	k := &tplink.KLAPClient{Base: "http://" + p.Addr(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: hc}
	if err := k.Handshake(ctx); err == nil {
		t.Error("KLAP was taken in TPAP mode")
	}

	bad := &tplink.TPAPClient{Base: "http://" + p.Addr(), Password: "wrong", HTTP: hc}
	for i := 0; i < 3; i++ {
		if err := bad.Handshake(ctx); !errors.Is(err, tplink.ErrTPAPAuth) {
			t.Fatalf("wrong password %d: %v", i, err)
		}
	}
	if err := bad.Handshake(ctx); !errors.Is(err, tplink.ErrTPAPLocked) {
		t.Errorf("after three failures: %v", err)
	}
	if _, err := tplink.TPAPDiscover(ctx, hc, "http://"+p.Addr()); !errors.Is(err, tplink.ErrTPAPLocked) {
		t.Errorf("discover of a locked device: %v", err)
	}
	p.UnlockTPAP()
	c2 := &tplink.TPAPClient{Base: "http://" + p.Addr(), Password: sim.DemoPassword, HTTP: hc}
	if err := c2.Handshake(ctx); err != nil {
		t.Errorf("after the lock: %v", err)
	}
}

// A KLAP-mode device does not answer discover as a TPAP device.
//
// Covers: SIG-IMP-179
func TestTPAPDiscoverKLAPDevice(t *testing.T) {
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, tpapPlugInfo(), nil)
	defer p.Close()
	if info, err := tplink.TPAPDiscover(ctx, &http.Client{Timeout: 5 * time.Second}, "http://"+p.Addr()); info != nil || err != nil {
		t.Errorf("%v %v", info, err)
	}
}

// The 20002 answer of a device in TPAP mode leads to TPAPClient.
//
// Covers: SIG-IMP-179
func TestTPAPDiscovery20002(t *testing.T) {
	p := sim.StartTapoTPAP(sim.DemoUser, sim.DemoPassword, tpapPlugInfo(), nil)
	defer p.Close()
	r, err := sim.StartResponder(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found, err := tplink.TapoDiscover(ctx, r.Addr(), time.Second)
	if err != nil || len(found) != 1 {
		t.Fatalf("%v %v", found, err)
	}
	f := found[0]
	if f.Protocol() != "tpap" || f.TPAP == nil {
		t.Fatalf("protocol %q %+v", f.Protocol(), f.TPAP)
	}
	c := &tplink.TPAPClient{Base: f.BaseURL(), Password: sim.DemoPassword, HTTP: &http.Client{Timeout: 5 * time.Second}}
	if _, err := c.Call(ctx, "get_device_info", nil); err != nil {
		t.Errorf("%s: %v", f.BaseURL(), err)
	}
}

// A camera on TPAP over HTTPS: camera requests and the typed helpers, a
// wrong password, and its HTTPS login still taken.
//
// Covers: SIG-IMP-179
func TestTPAPCamera(t *testing.T) {
	cam := sim.StartCameraTPAP("admin", "camera-pass", camInfo())
	defer cam.Close()
	cam.Do(func() { cam.CamSections = camSections() })
	hc := tplink.InsecureHTTP(5 * time.Second)
	info := tplink.TPAPCameraInfo()
	c := &tplink.TPAPClient{Base: info.BaseURL(cam.Addr()), Password: "camera-pass", Camera: true, HTTP: hc}
	cm := tplink.TapoCamera{Caller: c}
	ci, err := cm.Info(ctx)
	if err != nil || ci.Model != "C210" {
		t.Fatalf("info %+v %v", ci, err)
	}
	if err := cm.SetLED(ctx, false); err != nil {
		t.Fatal(err)
	}
	if on, err := cm.LED(ctx); err != nil || on {
		t.Errorf("LED %v %v", on, err)
	}
	if cam.TPAPLogins() != 1 {
		t.Errorf("logins %d", cam.TPAPLogins())
	}
	bad := &tplink.TPAPClient{Base: "https://" + cam.Addr(), Password: "wrong", Camera: true, HTTP: hc}
	if err := bad.Handshake(ctx); !errors.Is(err, tplink.ErrTPAPAuth) {
		t.Errorf("wrong password: %v", err)
	}
	s := &tplink.SSLAESClient{Base: "https://" + cam.Addr(), Username: "admin", Password: "camera-pass", HTTP: hc}
	if _, err := tplink.CamDeviceInfo(ctx, s); err != nil {
		t.Errorf("the HTTPS login next to TPAP: %v", err)
	}
}
