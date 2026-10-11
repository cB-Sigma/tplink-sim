// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

var ctx = context.Background()

// The legacy protocol over TCP (a request, a module error) and over UDP
// (discovery).
func TestKasaLegacy(t *testing.T) {
	k, err := sim.StartKasa(map[string]any{"alias": "Kettle", "model": "HS110(EU)", "mac": "5C:62:8B:00:00:10", "mic_type": "IOT.SMARTPLUGSWITCH", "relay_state": 0.0},
		map[string]any{"power_mw": 1500.0})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	m, err := tplink.KasaRequest(ctx, k.Addr(), map[string]any{"system": map[string]any{"set_relay_state": map[string]any{"state": 1}}}, 2*time.Second)
	if err != nil || tplink.KasaError(m) != nil || k.Get("relay_state") != 1.0 {
		t.Fatalf("set_relay_state: %v %v relay %v", m, err, k.Get("relay_state"))
	}
	m, err = tplink.KasaRequest(ctx, k.Addr(), map[string]any{"system": map[string]any{"no_such_method": map[string]any{}}}, 2*time.Second)
	if err != nil || tplink.KasaError(m) == nil {
		t.Errorf("unknown method: %v %v", m, err)
	}
	found, err := tplink.KasaDiscover(ctx, k.Addr(), 500*time.Millisecond)
	if err != nil || len(found) != 1 || found[0].Addr != k.Addr() || found[0].Sysinfo["alias"] != "Kettle" {
		t.Errorf("discovery: %+v %v", found, err)
	}
}

// The KLAP v2 handshake and requests on one session; a device that forgot
// the session; a wrong password.
//
// Covers: SIG-IPE-4447
func TestKLAPHandshake(t *testing.T) {
	auth := tplink.AuthHash("user@example.invalid", "pw")
	dev := sim.StartTapo("user@example.invalid", "pw", map[string]any{"device_on": true, "type": "SMART.TAPOPLUG"}, nil)
	defer dev.Close()
	c := &tplink.KLAPClient{Base: "http://" + dev.Addr(), Auth: auth, HTTP: &http.Client{Timeout: 2 * time.Second}}
	for i := 0; i < 3; i++ { // the sequence advances on one session
		info, err := c.Call(ctx, "get_device_info", nil)
		if err != nil || info["device_on"] != true {
			t.Fatalf("call %d: %v %v", i, info, err)
		}
	}
	dev.ForgetSessions() // a reboot: the client does the handshake again
	if _, err := c.Call(ctx, "get_device_info", nil); err != nil {
		t.Fatalf("after the device forgot the session: %v", err)
	}
	wrong := &tplink.KLAPClient{Base: "http://" + dev.Addr(), Auth: tplink.AuthHash("user@example.invalid", "other"), HTTP: http.DefaultClient}
	if _, err := wrong.Call(ctx, "get_device_info", nil); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("wrong password: %v", err)
	}
}

// KLAP v1 carries the legacy Kasa JSON; KLAPAuths finds the version and
// the account from the device's proof; blank credentials are tried too.
func TestKLAPAuths(t *testing.T) {
	d, k := sim.StartKasaKLAP(sim.DemoUser, sim.DemoPassword, 1, map[string]any{"alias": "Plug", "mac": "5C:62:8B:00:00:A1", "relay_state": 1.0}, nil)
	defer d.Close()
	c := &tplink.KLAPClient{Base: "http://" + d.Addr(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: &http.Client{Timeout: 2 * time.Second}}
	if _, err := c.Call(ctx, "get_device_info", nil); !errors.Is(err, tplink.ErrNotSmart) {
		t.Errorf("a Tapo request to a Kasa device: %v", err)
	}
	m, err := c.KasaCall(ctx, map[string]any{"system": map[string]any{"get_sysinfo": map[string]any{}}})
	if err != nil {
		t.Fatal(err)
	}
	if info := m["system"].(map[string]any)["get_sysinfo"].(map[string]any); info["alias"] != "Plug" {
		t.Errorf("sysinfo %v", info)
	}
	if _, err := c.KasaCall(ctx, map[string]any{"system": map[string]any{"set_relay_state": map[string]any{"state": 0}}}); err != nil || k.Get("relay_state") != 0.0 {
		t.Errorf("set_relay_state: %v, relay %v", err, k.Get("relay_state"))
	}
	if a := c.Matched(); a.Version != 1 || a.Name != "account" {
		t.Errorf("matched %+v", a)
	}

	blank := sim.StartTapo("", "", map[string]any{"device_on": true}, nil)
	defer blank.Close()
	b := &tplink.KLAPClient{Base: "http://" + blank.Addr(), Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword), HTTP: http.DefaultClient}
	if err := b.Handshake(ctx); err != nil || b.Matched().Name != "blank" {
		t.Errorf("blank credentials: %v %+v", err, b.Matched())
	}
	other := &tplink.KLAPClient{Base: "http://" + blank.Addr(), Auth: tplink.AuthHash("someone@example.invalid", "x"), HTTP: http.DefaultClient}
	if err := other.Handshake(ctx); !errors.Is(err, tplink.ErrKLAPAuth) {
		t.Errorf("a wrong account: %v", err)
	}
}

// securePassthrough with login version 2 and 1, a refused account, and a
// session the device forgot.
func TestSecurePassthrough(t *testing.T) {
	for _, lv := range []int{1, 2} {
		d := sim.StartTapoAES(sim.DemoUser, sim.DemoPassword, lv, map[string]any{"device_on": false, "model": "P100"}, nil)
		defer d.Close()
		a := &tplink.AESClient{Base: "http://" + d.Addr(), Logins: tplink.AESLogins(sim.DemoUser, sim.DemoPassword, lv), HTTP: &http.Client{Timeout: 5 * time.Second}}
		if err := a.Handshake(ctx); err != nil {
			t.Fatalf("v%d handshake: %v", lv, err)
		}
		if _, err := a.Call(ctx, "set_device_info", map[string]any{"device_on": true}); err != nil {
			t.Fatalf("v%d set: %v", lv, err)
		}
		if d.Get("device_on") != true || a.Matched() == "" {
			t.Errorf("v%d: device_on %v matched %q", lv, d.Get("device_on"), a.Matched())
		}
		d.ForgetAES()
		if info, err := a.Call(ctx, "get_device_info", nil); err != nil || info["model"] != "P100" {
			t.Errorf("v%d after the device forgot the session: %v %v", lv, info, err)
		}
		if _, err := a.Call(ctx, "no_such_method", nil); !tplink.IsUnsupported(err) {
			t.Errorf("v%d unknown method: %v", lv, err)
		}
		wrong := &tplink.AESClient{Base: "http://" + d.Addr(), Logins: tplink.AESLogins(sim.DemoUser, "other", lv), HTTP: http.DefaultClient}
		if _, err := wrong.Call(ctx, "get_device_info", nil); !errors.Is(err, tplink.ErrKLAPAuth) {
			t.Errorf("v%d wrong password: %v", lv, err)
		}
	}
}

// A hub's children are read page by page; a request reaches one child.
func TestHubChildren(t *testing.T) {
	h := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"type": "SMART.TAPOHUB", "model": "H100"}, nil)
	defer h.Close()
	for i := 0; i < 7; i++ {
		h.Children = append(h.Children, map[string]any{"device_id": "child" + string(rune('a'+i)), "category": "subg.trigger.temp-hmdt-sensor", "current_temp": 21.0 + float64(i)})
	}
	h.PageSize = 3
	c := &tplink.KLAPClient{Base: "http://" + h.Addr(), Auth: tplink.AuthHash(sim.DemoUser, sim.DemoPassword), HTTP: http.DefaultClient}
	kids, err := tplink.ChildList(ctx, c)
	if err != nil || len(kids) != 7 || kids[6]["device_id"] != "childg" {
		t.Fatalf("children %d %v", len(kids), err)
	}
	info, err := tplink.ControlChild(ctx, c, "childc", "get_device_info", nil)
	if err != nil || info["current_temp"] != 23.0 {
		t.Errorf("child info %v %v", info, err)
	}
	if _, err := tplink.ControlChild(ctx, c, "childc", "no_such_method", nil); !tplink.IsUnsupported(err) {
		t.Errorf("unknown child method: %v", err)
	}
}

// The 20002 probe reaches the responder, and each device's answer is read.
func TestTapoDiscover(t *testing.T) {
	p := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"device_id": "80223F", "type": "SMART.TAPOPLUG", "model": "P110", "mac": "3C-52-A1-00-00-04"}, nil)
	defer p.Close()
	b := sim.StartTapoAES(sim.DemoUser, sim.DemoPassword, 2, map[string]any{"device_id": "80223G", "type": "SMART.TAPOBULB", "model": "L530", "mac": "3C-52-A1-00-00-05"}, nil)
	defer b.Close()
	r, err := sim.StartResponder(p, b)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	found, err := tplink.TapoDiscover(ctx, r.Addr(), 600*time.Millisecond)
	if err != nil || len(found) != 2 {
		t.Fatalf("found %+v %v", found, err)
	}
	protos := map[string]string{}
	for _, f := range found {
		protos[f.MAC] = f.Protocol()
	}
	if protos["3c52a1000004"] != "klap" || protos["3c52a1000005"] != "aes" || r.Probes() == 0 {
		t.Errorf("protocols %v, probes %d", protos, r.Probes())
	}
}
