# tplink-sim

Simulated TP-Link Kasa and Tapo devices on their local protocols, for testing Kasa and Tapo clients without devices; not affiliated with TP-Link.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/tplink-sim.svg)](https://pkg.go.dev/github.com/cB-Sigma/tplink-sim) [![test](https://github.com/cB-Sigma/tplink-sim/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/tplink-sim/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

Kasa devices on the legacy protocol (TCP and UDP on one port), Kasa and
Tapo devices on KLAP v1 and v2, Tapo devices on securePassthrough with
either login version, hubs and power strips with children and button
logs, and a responder to the UDP 20002 discovery probe. Enough of each
device is answered for python-kasa's Device API to connect, read and
switch it. It is built on [tplink](https://github.com/cB-Sigma/tplink).
Written for SigmaM2M, a oneM2M platform; it has no dependency on it.

## What is simulated

| Feature | Status |
|---|---|
| Kasa legacy protocol: get_sysinfo, set_relay_state (also per child), dimmer brightness, bulb and light-strip light state, the energy meter (also per strip outlet), the time module; module errors for the rest | yes |
| KLAP v1 and v2: the two-step handshake, signed AES-128-CBC requests, sessions that end (`ForgetSessions`), answers with a wrong signature on demand (`BadAnswerSignature`) | yes |
| Kasa devices on KLAP carrying the legacy JSON (`StartKasaKLAP`) | yes |
| securePassthrough: RSA handshake, login version 1 or 2, tokens, sessions that end (`ForgetAES`) | yes |
| Tapo requests: get_device_info, set_device_info, get_energy_usage, get_current_power, get_device_time, component_nego, multipleRequest | yes |
| Hubs and strips: get_child_device_list paged, control_child, button trigger logs (`Press`) | yes |
| UDP 20002 responder checking the probe's header, CRC and key | yes; one answer per simulated device, from one port |
| HTTPS transports, cameras, the cloud, firmware update, schedules | no |

## Install

```
go get github.com/cB-Sigma/tplink-sim@latest
```

Requires Go 1.22 or later. The command:

```
go install github.com/cB-Sigma/tplink-sim/cmd/tplinksim@latest
```

## Command line

```
tplinksim -aes
```

| Flag | Default | Meaning |
|---|---|---|
| `-aes` | off | also serve a Tapo P100 plug on securePassthrough (older firmware) |

It serves a Kasa HS110 plug with an energy meter, an HS300 strip with
three outlets and a KL130 colour bulb on the legacy protocol, a Tapo P110
plug and an L530 bulb on KLAP v2, each on its own port of 127.0.0.1, and
the 20002 responder; it prints their addresses and the demo account.

## In Go tests

From `Example` in example_test.go:

```go
plug := sim.StartTapo(sim.DemoUser, sim.DemoPassword, map[string]any{"model": "P110", "device_on": false}, nil)
defer plug.Close()

c := &tplink.KLAPClient{
	Base:  "http://" + plug.Addr(), // a real device: http://<ip>
	Auths: tplink.KLAPAuths(sim.DemoUser, sim.DemoPassword),
	HTTP:  &http.Client{Timeout: 5 * time.Second},
}
_, err := c.Call(ctx, "set_device_info", map[string]any{"device_on": true})
info, _ := c.Call(ctx, "get_device_info", nil)
// info["device_on"] is true; c.Matched().Version is 2
```

## Documentation

Reference: https://pkg.go.dev/github.com/cB-Sigma/tplink-sim

| Package | What it does |
|---|---|
| `sim` | `StartKasa`, `StartKasaKLAP`, `StartTapo`, `StartTapoAES` (`Tapo`: `Children`, `PageSize`, `Press`, `Child`, `SetChild`, `Methods`), `StartResponder`, `NewDemo`, `DemoUser`, `DemoPassword` |
| `cmd/tplinksim` | Serves the demo devices and the responder |
| `interop` (nested module, tests only) | python-kasa's client against the simulators, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_TPLINK_INTEROP=1 go test -v ./...   # needs Docker
```

The tests at the module root drive the tplink clients against the
simulators: the legacy protocol, KLAP v1 and v2 with every credential
kind, securePassthrough, children and discovery. Tests carry `// Covers:
SIG-...` comments: requirement IDs of the SigmaM2M project's compliance
report, with no meaning outside it.

### Interop tested against

- python-kasa 0.11.0.1's client (2026-10-11, python:3.12.10-slim in
  Docker Desktop on arm64; `interop`), through its protocol classes and
  its Device API. Passed:
  - legacy protocol (HS110): get_sysinfo, set_relay_state, `Device`
    connect, update and turn_on, and its UDP discovery finding the plug;
  - KLAP v1 with the Kasa JSON (HS100 hardware 4.1): queries, `Device`
    connect and turn_on;
  - KLAP v2 (P110): get_device_info, set_device_info, `Device` connect
    and turn_off;
  - securePassthrough with login version 2 and 1 (P100): the same;
  - 20002 discovery of a KLAP and of a securePassthrough device, each
    then read over the protocol its answer named.
- The first runs found requests python-kasa's Device API needs that the
  simulators did not answer (multipleRequest, component_nego,
  get_device_time on Tapo; the time module on Kasa); they were added in
  v0.1.0.
- Not yet: compared with a real Kasa or Tapo device. The simulators and
  the tplink clients are written by the same author, so their agreement
  is not independent; the python-kasa runs are.

## Demo values

Every account, MAC address, device ID and address in this module is a
made-up demo value. Do not use them for real devices.

## Related repositories

- [tplink](https://github.com/cB-Sigma/tplink): the Kasa and Tapo
  clients this module is built on.

## Status

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam. TP-Link, Kasa and Tapo are trademarks of their owner; the
names are used only to say which devices this module simulates.
