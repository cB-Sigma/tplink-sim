// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"errors"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

// KLAP over HTTPS: the same client with an https base, found by
// discovery with is_support_https.
func TestKLAPOverHTTPS(t *testing.T) {
	d := sim.StartTapoHTTPS(sim.DemoUser, sim.DemoPassword, map[string]any{"device_id": "80223K", "model": "P110M", "type": "SMART.TAPOPLUG",
		"mac": "3C-52-A1-00-00-09", "device_on": false}, nil)
	defer d.Close()
	r, err := sim.StartResponder(d)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found, err := tplink.TapoDiscover(ctx, r.Addr(), 600*time.Millisecond)
	if err != nil || len(found) != 1 || !found[0].HTTPS || found[0].BaseURL() != "https://"+d.Addr() {
		t.Fatalf("found %+v %v", found, err)
	}
	c := &tplink.KLAPClient{Base: found[0].BaseURL(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if err := (tplink.TapoDevice{Caller: c}).SetOn(ctx, true); err != nil || d.Get("device_on") != true {
		t.Errorf("switch %v", err)
	}
}

func camInfo() map[string]any {
	return map[string]any{"device_type": "SMART.IPCAMERA", "device_model": "C210", "device_alias": "Porch", "sw_version": "1.3.11 Build 231117",
		"hw_version": "2.0", "mac": "00-31-92-00-00-0A", "dev_id": "80223CAM", "hw_id": "H1", "oem_id": "O1", "device_info": "C210 2.0 IPC"}
}

// The camera transport: both login steps, signed requests, a session the
// device forgot, a wrong password, the MD5 hash and the plain login.
func TestCameraTransport(t *testing.T) {
	cam := sim.StartCamera("admin", "camera-pass", camInfo())
	defer cam.Close()
	cam.CamSections = map[string]map[string]any{"lens_mask": {"lens_mask_info": map[string]any{"enabled": "off"}}}
	c := &tplink.SSLAESClient{Base: "https://" + cam.Addr(), Username: "admin", Password: "camera-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	info, err := tplink.CamDeviceInfo(ctx, c)
	if err != nil || info["device_model"] != "C210" {
		t.Fatalf("device info %v %v", info, err)
	}
	for i := 0; i < 3; i++ { // the sequence moves on
		if _, err := tplink.CamGet(ctx, c, "getLensMaskConfig", map[string]any{"lens_mask": map[string]any{"name": []any{"lens_mask_info"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tplink.CamGet(ctx, c, "setLensMaskConfig", map[string]any{"lens_mask": map[string]any{"lens_mask_info": map[string]any{"enabled": "on"}}}); err != nil {
		t.Fatal(err)
	}
	res, _ := tplink.CamGet(ctx, c, "getLensMaskConfig", map[string]any{"lens_mask": map[string]any{"name": []any{"lens_mask_info"}}})
	if res["lens_mask"].(map[string]any)["lens_mask_info"].(map[string]any)["enabled"] != "on" {
		t.Errorf("lens mask %v", res)
	}
	if _, err := tplink.CamGet(ctx, c, "getNoSuchThing", map[string]any{"nothing": map[string]any{}}); !tplink.IsUnsupported(err) {
		t.Errorf("unknown method %v", err)
	}
	cam.ForgetCameraSessions()
	if _, err := tplink.CamDeviceInfo(ctx, c); err != nil {
		t.Errorf("after the device forgot the session: %v", err)
	}
	if cam.TagFailures() != 0 {
		t.Errorf("tag failures %d", cam.TagFailures())
	}
	wrong := &tplink.SSLAESClient{Base: "https://" + cam.Addr(), Username: "admin", Password: "other", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if err := wrong.Handshake(ctx); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("wrong password: %v", err)
	}
	stranger := &tplink.SSLAESClient{Base: "https://" + cam.Addr(), Username: "someone", Password: "camera-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if err := stranger.Handshake(ctx); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("wrong user: %v", err)
	}

	old := sim.StartCamera("admin", "camera-pass", camInfo())
	defer old.Close()
	old.PasswordMD5 = true
	c2 := &tplink.SSLAESClient{Base: "https://" + old.Addr(), Username: "admin", Password: "camera-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if _, err := tplink.CamDeviceInfo(ctx, c2); err != nil {
		t.Errorf("MD5 firmware %v", err)
	}
	plain := sim.StartCamera("admin", "camera-pass", camInfo())
	defer plain.Close()
	plain.PlainLogin = true
	c3 := &tplink.SSLAESClient{Base: "https://" + plain.Addr(), Username: "admin", Password: "camera-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if _, err := tplink.CamDeviceInfo(ctx, c3); err != nil {
		t.Errorf("plain login %v", err)
	}
}

// An H200 hub on the camera transport: children and a request to one.
func TestH200Children(t *testing.T) {
	info := camInfo()
	info["device_type"], info["device_model"] = "SMART.TAPOHUB", "H200"
	h := sim.StartCamera("admin", "hub-pass", info)
	defer h.Close()
	h.Components = map[string]int{"childControl": 1}
	h.Children = []map[string]any{{"device_id": "T1", "model": "T315", "category": "subg.trigger.temp-hmdt-sensor", "current_temp": 21.0, "current_humidity": 50.0}}
	c := &tplink.SSLAESClient{Base: "https://" + h.Addr(), Username: "admin", Password: "hub-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	kids, err := tplink.CamChildList(ctx, c)
	if err != nil || len(kids) != 1 || kids[0]["model"] != "T315" {
		t.Fatalf("children %v %v", kids, err)
	}
	if r, err := tplink.CamControlChild(ctx, c, "T1", "get_device_info", nil); err != nil || r["current_temp"] != 21.0 {
		t.Errorf("child %v %v", r, err)
	}
	if _, err := tplink.CamControlChild(ctx, c, "T1", "no_such", nil); !tplink.IsUnsupported(err) {
		t.Errorf("child unknown method %v", err)
	}
	r, err := sim.StartResponder(h)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found, err := tplink.TapoDiscover(ctx, r.Addr(), 600*time.Millisecond)
	if err != nil || len(found) != 1 || found[0].Protocol() != "aes" || !found[0].HTTPS || found[0].Model != "H200" || found[0].LoginVersion != 3 {
		t.Errorf("found %+v %v", found, err)
	}
}

// A Kasa camera on its HTTPS transport, through KasaDevice.
func TestKasaCameraLinkie(t *testing.T) {
	k := sim.StartKasaCamera("cam-user", "cam-pass", map[string]any{"alias": "Garage", "model": "KC100(US)", "mic_type": "IOT.IPCAMERA", "relay_state": 1.0})
	defer k.Close()
	d, info, err := tplink.NewKasaDevice(ctx, &tplink.LinkieClient{Base: "https://" + k.Addr(), Username: "cam-user", Password: "cam-pass", HTTP: tplink.InsecureHTTP(5 * time.Second)})
	if err != nil || info["model"] != "KC100(US)" {
		t.Fatalf("sysinfo %v %v", info, err)
	}
	if err := d.SetAlias(ctx, "Drive"); err != nil || k.Get("alias") != "Drive" {
		t.Errorf("alias %v", err)
	}
	wrong := &tplink.LinkieClient{Base: "https://" + k.Addr(), Username: "cam-user", Password: "other", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if _, err := wrong.KasaCall(ctx, map[string]any{"system": map[string]any{"get_sysinfo": map[string]any{}}}); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("wrong password %v", err)
	}
}

// A robot vacuum on the plain HTTPS transport, through TapoDevice.
func TestTapoPlainHTTPS(t *testing.T) {
	v := sim.StartTapoSSL(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "RV30 Max", "type": "SMART.TAPOROBOVAC", "nickname": "VmFj"})
	defer v.Close()
	d := tplink.TapoDevice{Caller: &tplink.SSLClient{Base: "https://" + v.Addr(), Username: sim.DemoUser, Password: sim.DemoPassword, HTTP: tplink.InsecureHTTP(5 * time.Second)}}
	info, err := d.Info(ctx)
	if err != nil || info.Model != "RV30 Max" || info.Nickname != "Vac" {
		t.Fatalf("info %+v %v", info, err)
	}
	wrong := &tplink.SSLClient{Base: "https://" + v.Addr(), Username: sim.DemoUser, Password: "other", HTTP: tplink.InsecureHTTP(5 * time.Second)}
	if _, err := wrong.Call(ctx, "get_device_info", nil); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("wrong password %v", err)
	}
}
