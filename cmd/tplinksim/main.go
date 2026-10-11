// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Command tplinksim serves simulated TP-Link devices on 127.0.0.1, so a
// Kasa or Tapo client can be tried without devices:
//
//	Kasa HS110 plug with an energy meter   legacy protocol, TCP and UDP
//	Kasa HS300 strip with three outlets    legacy protocol
//	Kasa KL130 colour bulb                 legacy protocol
//	Tapo P110 plug with energy use         KLAP v2
//	Tapo L530 colour bulb                  KLAP v2
//	Kasa HS220 dimmer                      legacy protocol
//	Kasa KL430 light strip                 legacy protocol
//	Tapo H100 hub with three sensors       KLAP v2
//	Tapo C210 camera                       HTTPS (camera transport)
//	Tapo P100 plug on older firmware       securePassthrough (with -aes)
//
// and a responder to the UDP 20002 discovery probe for the Tapo devices.
// Each device has its own port (real devices use 9999 and 80 on their own
// addresses). The Tapo account is the demo account printed at start. For
// example:
//
//	tplinksim -aes
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cB-Sigma/tplink-sim/sim"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, nil); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "tplinksim:", err)
		os.Exit(1)
	}
}

// running is what the runner serves.
type running struct {
	demo      *sim.Demo
	aes       *sim.Tapo
	tpap      *sim.Tapo
	responder *sim.Responder
}

// run serves until ctx ends. ready, when not nil, receives what runs once
// it listens.
func run(ctx context.Context, args []string, out io.Writer, ready func(running)) error {
	fs := flag.NewFlagSet("tplinksim", flag.ContinueOnError)
	fs.SetOutput(out)
	withAES := fs.Bool("aes", false, "also serve a Tapo P100 plug on securePassthrough (older firmware)")
	withTPAP := fs.Bool("tpap", false, "also serve a Tapo P110M plug on TPAP (newer firmware, third-party compatibility off)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %v", fs.Args())
	}
	d, err := sim.NewDemo()
	if err != nil {
		return err
	}
	defer d.Close()
	r := running{demo: d}
	tapo := []*sim.Tapo{d.TapoPlug, d.TapoBulb, d.TapoHub, d.Camera}
	if *withAES {
		r.aes = sim.StartTapoAES(sim.DemoUser, sim.DemoPassword, 2, map[string]any{"device_id": "80223H", "model": "P100", "type": "SMART.TAPOPLUG",
			"fw_ver": "1.1.3 Build 210521", "mac": "3C-52-A1-00-00-06", "nickname": "UGx1Zw==", "device_on": false}, nil)
		defer r.aes.Close()
		tapo = append(tapo, r.aes)
	}
	if *withTPAP {
		r.tpap = sim.StartTapoTPAP(sim.DemoUser, sim.DemoPassword, map[string]any{"device_id": "80223T", "model": "P110M", "type": "SMART.TAPOPLUG",
			"fw_ver": "1.4.0 Build 250101", "mac": "3C-52-A1-00-00-07", "nickname": "UGx1Zw==", "device_on": false}, nil)
		defer r.tpap.Close()
		tapo = append(tapo, r.tpap)
	}
	if r.responder, err = sim.StartResponder(tapo...); err != nil {
		return err
	}
	defer r.responder.Close()
	fmt.Fprintf(out, "tplinksim: Tapo account %s / %s\n", sim.DemoUser, sim.DemoPassword)
	fmt.Fprintf(out, "tplinksim: camera user %s / %s\n", sim.DemoCameraUser, sim.DemoCameraPassword)
	for _, x := range []struct{ name, addr string }{
		{"Kasa HS110 plug (legacy)", d.Plug.Addr()}, {"Kasa HS300 strip (legacy)", d.Strip.Addr()}, {"Kasa KL130 bulb (legacy)", d.Bulb.Addr()},
		{"Kasa HS220 dimmer (legacy)", d.Dimmer.Addr()}, {"Kasa KL430 strip (legacy)", d.LightStrip.Addr()},
		{"Tapo P110 plug (KLAP)", d.TapoPlug.Addr()}, {"Tapo L530 bulb (KLAP)", d.TapoBulb.Addr()}, {"Tapo H100 hub (KLAP)", d.TapoHub.Addr()},
		{"Tapo C210 camera (HTTPS)", d.Camera.Addr()},
	} {
		fmt.Fprintf(out, "tplinksim: %-26s %s\n", x.name, x.addr)
	}
	if r.aes != nil {
		fmt.Fprintf(out, "tplinksim: %-26s %s\n", "Tapo P100 plug (AES)", r.aes.Addr())
	}
	if r.tpap != nil {
		fmt.Fprintf(out, "tplinksim: %-26s %s\n", "Tapo P110M plug (TPAP)", r.tpap.Addr())
	}
	fmt.Fprintf(out, "tplinksim: Tapo discovery probes to udp %s\n", r.responder.Addr())
	if ready != nil {
		ready(r)
	}
	<-ctx.Done()
	return nil
}
