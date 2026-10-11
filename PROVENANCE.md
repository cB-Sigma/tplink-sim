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

## Sources

| Area (files) | Followed |
|---|---|
| Simulators (sim/sim.go, sim/smart.go) | the tplink module's framing, sessions and probe; device behaviour from python-kasa's descriptions (facts only) |
| Command (cmd/tplinksim) | none (flags are our own) |
| Interop peer (interop/peer/client.py) | our own script calling python-kasa 0.11.0.1, installed at test time |

No specification or vendor text is reproduced.

## Tests

| Test | Source |
|---|---|
| client_test.go, example_test.go, sim/, cmd/tplinksim | the tplink clients against these simulators, written from the same descriptions; not a real device |
| interop/ | python-kasa 0.11.0.1's client against these simulators |

All accounts, MAC addresses, device IDs and addresses are made-up demo
values.
