# tplink-sim

Simulated TP-Link Kasa and Tapo devices on their local protocols, for testing Kasa and Tapo clients without devices; not affiliated with TP-Link.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/tplink-sim.svg)](https://pkg.go.dev/github.com/cB-Sigma/tplink-sim) [![test](https://github.com/cB-Sigma/tplink-sim/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/tplink-sim/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

Kasa devices on the legacy protocol (TCP and UDP on one port), Kasa and
Tapo devices on KLAP v1 and v2 over HTTP and HTTPS, Tapo devices on
securePassthrough with either login version, Tapo devices and cameras on
the TPAP login of newer firmware, Tapo cameras and the H200
hub on their HTTPS transport, Kasa cameras, robot vacuums on the plain
HTTPS transport, hubs and power strips with children, sensors and button
logs, and a responder to the UDP 20002 discovery probe. Beyond switching,
the simulated devices keep energy statistics, rules, dimmer and light
settings, light effects, firmware updates and Wi-Fi set-up. Enough of
each device is answered for python-kasa's Device API to connect, read,
switch and change it. It is built on
[tplink](https://github.com/cB-Sigma/tplink). Written for SigmaM2M, a
oneM2M platform; it has no dependency on it.

## What is simulated

| Feature | Status |
|---|---|
| Kasa legacy protocol: get_sysinfo, set_relay_state (also per child), dimmer brightness, bulb and light-strip light state, the energy meter (also per strip outlet), the time module; module errors for the rest | yes |
| Kasa modules beyond switching, under the plug and bulb module names: energy day and month statistics and erasing them, runtime statistics, schedule, countdown and away-mode rules (add, edit, delete, enable; `Rules`), dimmer transitions, fades, ramp rate and parameters (`DimmerSetting`), light state, details and presets, light strip effects, LED, name (also per outlet), reboot (`Reboots`), cloud information and firmware list (`Firmware`), Wi-Fi scan (`Networks`) and join (`Joined`) | yes, since v0.2.0 |
| KLAP v1 and v2: the two-step handshake, signed AES-128-CBC requests, sessions that end (`ForgetSessions`), answers with a wrong signature on demand (`BadAnswerSignature`) | yes |
| KLAP over HTTPS (`StartTapoHTTPS`) | yes, since v0.2.0 |
| Kasa devices on KLAP carrying the legacy JSON (`StartKasaKLAP`) | yes |
| securePassthrough: RSA handshake, login version 1 or 2, tokens, sessions that end (`ForgetAES`) | yes |
| Tapo requests: get_device_info, set_device_info, get_energy_usage, get_current_power, get_device_time, component_nego, multipleRequest | yes |
| Tapo modules beyond switching (`TapoState`): extra components, settings answered and kept (auto-off, LED, transitions, automatic updates, light strip effect, ...), rule lists, presets, light effects, energy readings and hourly, daily and monthly history, device use, firmware offer, download and install in steps (`LatestFirmware`), Wi-Fi scan, set-up with a new account (`QuickSetup`), reboot | yes, since v0.2.0 |
| Hubs and strips: get_child_device_list paged, control_child (also with multipleRequest inside), children's components, sensor records, button trigger logs (`Press`) | yes; batches, components and records since v0.2.0 |
| Tapo cameras, doorbells and the H200 hub on the HTTPS transport (`StartCamera`): the nonce login with a SHA-256 or MD5 password hash (`PasswordMD5`) or the plain login (`PlainLogin`), request tags and sequence checked (`TagFailures`), sessions that end, camera requests from `CamSections`, renaming, H200 children | yes, since v0.2.0 |
| Kasa cameras (`StartKasaCamera`) and robot vacuums on the plain HTTPS transport (`StartTapoSSL`) | yes, since v0.2.0 |
| UDP 20002 responder checking the probe's header, CRC and key; the answer's encrypted part on demand (`DiscoverySecret`); HTTPS and camera answers | yes; one answer per simulated device, from one port |
| Tapo schedule and countdown writes (add, edit, remove; refusals for a full schedule, a rule at the same time, an unknown id, a second countdown) | yes, since v0.3.0 |
| Camera behaviour (`CamState`): pan and tilt (`CamPosition`), saved positions, the siren, the manual alarm (`CamAlarm`), recordings (`Recordings`), reboot and SD card format (`CamCounts`), Wi-Fi scan, the time zone; detection and other settings through `CamSections` | yes, since v0.3.0 |
| Robot vacuums (`StartVacuum`, `VacuumState`): status, cleaning by home or rooms, pause, return to dock (`Advance` moves time on), suction, water, passes, consumables, records, maps and rooms, mop, dock emptying, volume, carpet | yes, since v0.3.0 |
| The TPAP login of newer firmware: Tapo devices in TPAP mode (`StartTapoTPAP`: discover with the tpap object, SPAKE2+ with the account password, AES-128-CCM requests, KLAP refused with 403, the 20002 answer naming TPAP) and cameras that take TPAP next to their HTTPS login (`StartCameraTPAP`); wrong passwords (-2203, cameras -40401), the lock after failed logins (`TPAP.LockAfter`, `UnlockTPAP`), sessions that end (`ForgetTPAPSessions`), stale sequence numbers refused, `TPAPLogins` | yes, since v0.4.0 |
| Camera video, the cloud | no; see "Not yet" |

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
tplinksim -aes -tpap
```

| Flag | Default | Meaning |
|---|---|---|
| `-aes` | off | also serve a Tapo P100 plug on securePassthrough (older firmware) |
| `-tpap` | off | also serve a Tapo P110M plug on TPAP (newer firmware, third-party compatibility off); since v0.4.0 |

It serves a Kasa HS110 plug with an energy meter, an HS300 strip with
three outlets, a KL130 colour bulb, an HS220 dimmer and a KL430 light
strip on the legacy protocol, a Tapo P110 plug, an L530 bulb and an H100
hub with a temperature and humidity sensor, a button and a contact sensor
on KLAP v2, and a Tapo C210 camera on the HTTPS transport, each on its
own port of 127.0.0.1, and the 20002 responder; it prints their
addresses, the demo account and the demo camera user.

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
| `sim` | `StartKasa`, `StartKasaKLAP`, `StartKasaCamera` (`Kasa`: `KasaState`, `Do`, `Rules`, `Joined`, `Reboots`, `DimmerSetting`), `StartTapo`, `StartTapoAES`, `StartTapoHTTPS`, `StartTapoSSL`, `StartCamera`, `StartTapoTPAP`, `StartCameraTPAP` (`Tapo`: `TapoState`, `Do`, `Children`, `PageSize`, `Press`, `Child`, `SetChild`, `Methods`, `Setting`, `QuickSetup`, `TapoReboots`, `TagFailures`, `ForgetCameraSessions`, `CamPosition`, `CamAlarm`, `CamCounts`, `TPAP`, `TPAPLogins`, `ForgetTPAPSessions`, `UnlockTPAP`), `TPAPSettings`, `CamState`, `CamClip`, `StartVacuum`, `NewVacuumState`, `VacuumState`, `VacuumMap`, `VacuumRoom`, `VacuumComponents`, `StartResponder`, `NewDemo`, `DemoUser`, `DemoPassword`, `DemoCameraUser`, `DemoCameraPassword`, `DemoDayWh`, `DemoDayMinutes`, `DemoHourWh` |
| `cmd/tplinksim` | Serves the demo devices and the responder |
| `interop` (nested module, tests only) | python-kasa's client, and the tapo crate's Python bindings, against the simulators, in containers |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_TPLINK_INTEROP=1 go test -v ./...   # needs Docker
```

The tests at the module root drive the tplink clients against the
simulators: the legacy protocol, KLAP v1 and v2 with every credential
kind, KLAP over HTTPS, securePassthrough, the camera, Kasa camera and
plain HTTPS transports, TPAP (tpap_test.go), children and discovery, and
every typed helper (devices_test.go, https_test.go). Tests carry `// Covers:
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
- Added in v0.2.0, same image and python-kasa version, 2026-10-11, all
  passed, through python-kasa's Device API unless said:
  - Kasa plug (HS110): energy readings, daily and monthly energy
    statistics, daily runtime, schedule and away-mode rule lists, a rule
    deleted by python-kasa, LED, rename, erasing statistics, Wi-Fi scan
    and join, reboot;
  - Kasa dimmer (HS220): python-kasa's dimmer transition and fade time;
  - Kasa bulb (KL130) and light strip (KL430): colour with a transition,
    and one of python-kasa's built-in light strip effects set and read
    back;
  - Tapo plug (P110, energy component version 2, firmware, auto-off,
    LED, cloud connection): energy readings, python-kasa's firmware check
    and update run to its end (download, flash, the new version
    reported), auto-off delay and switch, LED, Wi-Fi scan, reboot;
  - Tapo bulb (L530, transitions version 3, light effects, presets):
    colour, the turn-on transition, a light effect (python-kasa first
    writes the bulb's brightness into it), the preset list;
  - Tapo hub (H100) with a T315 sensor, an S200B button and a T110
    contact sensor: temperature, humidity, the button's log, the contact
    state;
  - the 20002 answer's encrypted part, decrypted by python-kasa's
    discovery;
  - KLAP over HTTPS (P110M): protocol queries, connect and switch;
  - camera transport: a C210 camera (connect with the nonce login,
    every request's tag and sequence accepted, rename) and an H200 hub
    with a T315 child read through controlChild;
  - Kasa camera transport (protocol class; the simulator takes any login,
    as python-kasa sends its built-in default) and the plain HTTPS
    transport of robot vacuums (protocol class, a single request and a
    batch).
  These runs found two things the simulator lacked, added before the
  release: get_emeter_vgain_igain and edit_dynamic_light_effect_rule
  (with current_rule_id), and the camera clock section.
- Added in v0.3.0, 2026-10-12, Docker Desktop on arm64, all passed:
  - python-kasa 0.11.0.1's camera Device API against a simulated C210
    (image `sigma-tplink-sim-interop:python-kasa-0.11.0.1-r3`): it loaded
    the LED, lens mask, pan and tilt, motion, person, baby cry and tamper
    detection, alarm, last alert and time modules; read every feature;
    switched the LED and three detections, set the alarm volume, moved
    the motor (single "do" form), saved a position, sounded the siren and
    set the lens mask, each checked in the simulator. The first run
    found the siren answered with the module level, which python-kasa's
    alarm module cannot read; fixed.
  - python-kasa 0.11.0.1's vacuum Device API against a simulated vacuum
    on the plain HTTPS transport: device type and the clean, records,
    consumables, dustbin, mop and speaker modules; start, pause and
    return home followed in its status; fan speed, volume and emptying
    changed.
  - the tapo crate's Python bindings 0.11.1 (MIT; image
    `sigma-tplink-sim-tapo-interop:tapo-0.11.1`) against a simulated P110
    on KLAP v2: two schedules added (weekly clock, sunset), listed,
    edited (disabled, then a new time and state), the maximum read, the
    timer armed, read and cleared, one rule and then all removed; the
    tplink client read the rule tapo left exactly as tapo describes it,
    and tapo read a weekend sunrise rule the tplink client added.
- Added in v0.4.0, 2026-10-12, Docker Desktop on arm64, all passed:
  - the tapo crate's Python bindings 0.11.1 (image
    `sigma-tplink-sim-tapo-interop:tapo-0.11.1-r2`, with tpap.py) against
    a simulated P110 in TPAP mode, given only its address: tapo asked
    discover, chose TPAP, logged in (SPAKE2+ with the SHA-1 passcode,
    3000 PBKDF2 iterations, its check of the device's confirmation
    passed), read the device information, switched the plug on and off
    and read its energy use over AES-128-CCM; one login for the run. A
    wrong password was reported by tapo as TPAP_CREDENTIALS.
  - the same client against a simulated C220 camera that takes TPAP next
    to its HTTPS login: tapo's TPAP login (MD5 passcode, over HTTPS)
    succeeded without falling back, and it read the device information.
  - The first run found that the simulator answered set_device_info with
    an empty result, which tapo reads as a result that must carry
    "response"; the simulator now answers without a result. The
    python-kasa runs above were repeated with that change and passed.
  - python-kasa has no TPAP: neither its 0.11.0.1 release nor its master
    (38a3184, 2026-10-09); its TPAP pull request (#1592) is not merged and
    was not run. So the TPAP device side is checked against one
    independent client, the tapo crate.
- Not yet: compared with a real Kasa or Tapo device. The simulators and
  the tplink clients are written by the same author, so their agreement
  is not independent; the python-kasa runs are.

## Not yet

| Item | Why |
|---|---|
| Camera video and snapshots | streams belong outside these modules |
| Detections firing (motion events, alarms raised by detection) | detection settings are kept; nothing triggers them yet |
| Writing away (antitheft) rules | the client does not offer it (no open client sends the requests) |
| TPAP variants beyond the account password: the default passcode, cipher suites other than 1, session ciphers other than AES-128-CCM, the device certificate proof (dac) | the client does not offer them (see tplink's "Not yet"); the simulator answers suite 1, aes_128_ccm and userpw only |
| TPAP mode switched at run time (the app's third-party compatibility option turned on) | how a device answers TPAP messages in KLAP mode is not described publicly; start a KLAP device (`StartTapo`) instead |
| The TP-Link cloud | not a LAN protocol; needs an account |

## Demo values

Every account, MAC address, device ID and address in this module is a
made-up demo value. Do not use them for real devices.

## Related repositories

- [tplink](https://github.com/cB-Sigma/tplink): the Kasa and Tapo
  clients and helpers this module is built on.

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
