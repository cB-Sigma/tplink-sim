// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

func TestDemo(t *testing.T) {
	d, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	m, err := tplink.KasaRequest(ctx, d.Strip.Addr(), map[string]any{"system": map[string]any{"get_sysinfo": map[string]any{}}}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if kids := m["system"].(map[string]any)["get_sysinfo"].(map[string]any)["children"].([]any); len(kids) != 3 {
		t.Errorf("strip children %v", kids)
	}
	if len(d.Strip.Requests()) != 1 {
		t.Errorf("requests %v", d.Strip.Requests())
	}
	c := &tplink.KLAPClient{Base: "http://" + d.TapoBulb.Addr(), Auth: tplink.AuthHash(sim.DemoUser, sim.DemoPassword), HTTP: http.DefaultClient}
	if _, err := c.Call(ctx, "set_device_info", map[string]any{"brightness": 20}); err != nil || d.TapoBulb.Get("brightness") != 20.0 {
		t.Errorf("bulb brightness %v %v", d.TapoBulb.Get("brightness"), err)
	}
	if ms := d.TapoBulb.Methods(); len(ms) != 1 || ms[0] != "set_device_info" {
		t.Errorf("methods %v", ms)
	}
	// A button press appears in the child's trigger log.
	h := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"type": "SMART.TAPOHUB"}, nil)
	defer h.Close()
	h.Children = []map[string]any{{"device_id": "btn1", "category": "subg.trigger.button"}}
	h.Press("btn1", "singleClick")
	hc := &tplink.KLAPClient{Base: "http://" + h.Addr(), Auth: tplink.AuthHash(sim.DemoUser, sim.DemoPassword), HTTP: http.DefaultClient}
	logs, err := tplink.ControlChild(ctx, hc, "btn1", "get_trigger_logs", map[string]any{"page_size": 5, "start_id": 0})
	if l, _ := logs["logs"].([]any); err != nil || len(l) != 1 {
		t.Errorf("trigger logs %v %v", logs, err)
	}
}

func Example() {
	d, _ := sim.NewDemo() // three Kasa devices and two Tapo devices
	defer d.Close()
	fmt.Println(d.Plug.Get("alias"), "/", d.TapoPlug.Get("model"))
	// Output: Washing machine / P110
}
