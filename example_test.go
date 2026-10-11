// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tplink_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

// Switch a simulated Tapo plug on over KLAP.
func Example() {
	plug := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "P110", "device_on": false}, nil)
	defer plug.Close()

	c := &tplink.KLAPClient{
		Base:  "http://" + plug.Addr(), // a real device: http://<ip>
		Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword),
		HTTP:  &http.Client{Timeout: 5 * time.Second},
	}
	ctx := context.Background()
	_, err := c.Call(ctx, "set_device_info", map[string]any{"device_on": true})
	info, _ := c.Call(ctx, "get_device_info", nil)
	fmt.Println(err, info["model"], info["device_on"], "KLAP v", c.Matched().Version)
	// Output: <nil> P110 true KLAP v 2
}

// Read a simulated Kasa plug on the legacy protocol.
func ExampleKasaRequest() {
	k, _ := sim.StartKasa(map[string]any{"alias": "Kettle", "model": "HS110(EU)", "relay_state": 1.0}, nil)
	defer k.Close()
	m, err := tplink.KasaRequest(context.Background(), k.Addr(), // a real device: <ip>:9999
		map[string]any{"system": map[string]any{"get_sysinfo": map[string]any{}}}, 3*time.Second)
	info := m["system"].(map[string]any)["get_sysinfo"].(map[string]any)
	fmt.Println(err, info["alias"], info["relay_state"])
	// Output: <nil> Kettle 1
}
