// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

// The requests python-kasa's Device API needs, found missing by the
// python-kasa interop test: multipleRequest, component_nego and
// get_device_time on Tapo devices, the time module on Kasa devices.
func TestAnswersPythonKasaNeeds(t *testing.T) {
	ctx := context.Background()
	d := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "P110", "device_on": false}, map[string]any{"current_power": 5.0})
	defer d.Close()
	c := &tplink.KLAPClient{Base: "http://" + d.Addr(), Auth: tplink.AuthHash(sim.DemoUser, sim.DemoPassword), HTTP: &http.Client{Timeout: 2 * time.Second}}
	res, err := c.Call(ctx, "multipleRequest", map[string]any{"requests": []any{
		map[string]any{"method": "get_device_info"},
		map[string]any{"method": "set_device_info", "params": map[string]any{"device_on": true}},
		map[string]any{"method": "no_such_method"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rs, _ := res["responses"].([]any)
	if len(rs) != 3 {
		t.Fatalf("responses %v", res)
	}
	first, third := rs[0].(map[string]any), rs[2].(map[string]any)
	if first["method"] != "get_device_info" || first["error_code"] != 0.0 || first["result"].(map[string]any)["model"] != "P110" ||
		third["error_code"] != -1002.0 || d.Get("device_on") != true {
		t.Errorf("responses %v, device_on %v", rs, d.Get("device_on"))
	}
	nego, err := c.Call(ctx, "component_nego", nil)
	if l, _ := nego["component_list"].([]any); err != nil || len(l) != 3 {
		t.Errorf("component_nego %v %v", nego, err)
	}
	if tm, err := c.Call(ctx, "get_device_time", nil); err != nil || tm["timestamp"] == nil {
		t.Errorf("get_device_time %v %v", tm, err)
	}
	if p, err := c.Call(ctx, "get_current_power", nil); err != nil || p["current_power"] != 5.0 {
		t.Errorf("get_current_power %v %v", p, err)
	}

	k, err := sim.StartKasa(map[string]any{"alias": "Kettle", "relay_state": 0.0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	m, err := tplink.KasaRequest(ctx, k.Addr(), map[string]any{"time": map[string]any{"get_time": map[string]any{}, "get_timezone": map[string]any{}}}, 2*time.Second)
	if err != nil || tplink.KasaError(m) != nil {
		t.Fatalf("time %v %v", m, err)
	}
	tm := m["time"].(map[string]any)
	if tm["get_time"].(map[string]any)["year"] == nil || tm["get_timezone"].(map[string]any)["index"] != 39.0 {
		t.Errorf("time %v", tm)
	}
}
