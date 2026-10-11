# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.1.0 (2026-10-11)

First release as a module of its own, split from the tplink module:
simulated Kasa devices on the legacy protocol (`StartKasa`), Kasa and
Tapo devices on KLAP v1 and v2 (`StartKasaKLAP`, `StartTapo`), Tapo
devices on securePassthrough (`StartTapoAES`), hubs and strips with
children, the 20002 responder (`StartResponder`), `NewDemo` and
`cmd/tplinksim`.

- Added the requests python-kasa's Device API sends: `multipleRequest`,
  `component_nego`, `get_device_time` and `get_current_power` on Tapo
  devices, the `time` module on Kasa devices.
- The tplink clients' tests against the simulators moved here from
  tplink.
- Interop tests running python-kasa 0.11.0.1's client against the
  simulators, in a nested test-only module, `interop`.
