// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tplink"
	"github.com/cB-Sigma/tplink-sim/sim"
)

func TestRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan running, 1)
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, []string{"-aes"}, &out, func(r running) { got <- r }) }()
	var r running
	select {
	case r = <-got:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready")
	}
	found, err := tplink.TapoDiscover(ctx, r.responder.Addr(), 600*time.Millisecond)
	if err != nil || len(found) != 3 {
		t.Fatalf("discovery %+v %v", found, err)
	}
	a := &tplink.AESClient{Base: "http://" + r.aes.Addr(), Logins: tplink.AESLogins(sim.DemoUser, sim.DemoPassword, 2), HTTP: &http.Client{Timeout: 5 * time.Second}}
	if info, err := a.Call(ctx, "get_device_info", nil); err != nil || info["model"] != "P100" {
		t.Errorf("AES plug %v %v", info, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Tapo P100 plug (AES)") || !strings.Contains(out.String(), sim.DemoUser) {
		t.Errorf("output %s", out.String())
	}
}

func TestRunnerFlags(t *testing.T) {
	for _, args := range [][]string{{"-bogus"}, {"extra"}} {
		if err := run(context.Background(), args, &bytes.Buffer{}, nil); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}
