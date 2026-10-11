# Provenance

## How the code was written

Written for SigmaM2M, a oneM2M platform, in Go, by the author of the
tplink module and from the same public descriptions (facts only;
python-kasa is GPL-3.0 and no code was taken from it). The Kasa and KLAP
simulators came with SigmaM2M's first commit (2026-09-27); KLAP v1 with
the Kasa JSON, securePassthrough, hubs and strips with children and the
20002 responder were added on 2026-10-05. The simulators, their command
and the tests that drive them moved out of the tplink module into this
one on 2026-10-11 (with `git mv`).

On 2026-10-11 python-kasa 0.11.0.1's client was run against the
simulators (`interop`). Its protocol classes worked unchanged; its Device
API needed requests the simulators did not answer, which were added the
same day (`TestAnswersPythonKasaNeeds`):

- Tapo: `multipleRequest` (python-kasa batches its requests),
  `component_nego`, `get_device_time` and `get_current_power`.
- Kasa: the `time` module (`get_time`, `get_timezone`).

v0.2.0 (2026-10-11): sim/kasa_modules.go, sim/smart_modules.go,
sim/camera.go and sim/https_plain.go were written for this module, from
the same facts as tplink v0.2.0 (python-kasa 0.11.0.1's module, protocol
and transport files and recorded fixtures for request and answer shapes;
tapo-api and the tapo crate, both MIT, for the energy history fields;
facts only). The device behaviour behind the answers (statistics figures,
the steps of a firmware update, which settings exist by default) is our
own and made up for demos. python-kasa's client was then run against
them; it found three gaps, filled the same day: get_emeter_vgain_igain,
edit_dynamic_light_effect_rule with current_rule_id, and the camera
clock section. The python-kasa runs also showed a data race in the 20002
answer, fixed.

v0.3.0 (2026-10-12): sim/rules.go (schedule and countdown writes),
sim/camera_more.go (motor, presets, siren, manual alarm, recordings,
reboot, SD card format, Wi-Fi scan) and sim/vacuum.go (robot vacuum
state) were written for this module, from the same facts as tplink
v0.3.0 (the tapo crate 0.11.1 and plugp100 for rule writes; python-kasa's
smartcam and vacuum modules and fixtures, pytapo 3.4.26 and the tapo
crate for camera requests; facts only). The behaviour behind the answers
(rule ids, the one-countdown limit, how far a motor step turns, how a
clean progresses) is our own. python-kasa's client was run against the
camera and the vacuum, and the tapo crate's client against the rule
writes. The camera run found that the simulator answered the siren with
the module level, where recorded devices answer without it; fixed the
same day.

## Sources

| Area (files) | Followed |
|---|---|
| Simulators (sim/sim.go, sim/smart.go) | the tplink module's framing, sessions and probe; device behaviour from python-kasa's descriptions (facts only) |
| Modules and HTTPS devices (sim/kasa_modules.go, sim/smart_modules.go, sim/camera.go, sim/https_plain.go) | the tplink module's transports and hash functions; method names and answer shapes from python-kasa 0.11.0.1 and its fixtures, energy history fields from tapo-api and the tapo crate (facts only); figures and behaviour our own |
| Command (cmd/tplinksim) | none (flags are our own) |
| Rules, cameras, vacuums (sim/rules.go, sim/camera_more.go, sim/vacuum.go) | the tplink module's v0.3.0 facts (tapo crate, plugp100, python-kasa, pytapo); behaviour our own |
| Interop peer (interop/peer/client.py) | our own script calling python-kasa 0.11.0.1, installed at test time |
| Second interop peer (interop/tapopeer/schedule.py) | our own script calling the tapo package 0.11.1 (MIT), installed at test time |

No specification or vendor text is reproduced.

## Tests

| Test | Source |
|---|---|
| client_test.go, example_test.go, sim/, cmd/tplinksim | the tplink clients against these simulators, written from the same descriptions; not a real device |
| interop/ | python-kasa 0.11.0.1's client, and the tapo package 0.11.1's client, against these simulators |

All accounts, MAC addresses, device IDs and addresses are made-up demo
values.
