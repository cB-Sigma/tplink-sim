# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.4.0 (2026-10-12)

Additions only; the v0.3.0 API is unchanged. Requires tplink v0.4.0.

- TPAP, the login of newer firmware: `StartTapoTPAP` (a Tapo device in
  TPAP mode: discover with the tpap object, pake_register with
  password_shadow 2, pake_share, AES-128-CCM requests to /stok=.../ds,
  KLAP refused with 403, the 20002 answer naming TPAP with the tpap
  object) and `StartCameraTPAP` (a camera taking TPAP over HTTPS with the
  MD5 passcode next to its HTTPS login). `Tapo.TPAP` (`TPAPSettings`:
  iterations, lock after failed logins, first sequence number, announced
  port), `TPAPLogins`, `ForgetTPAPSessions`, `UnlockTPAP`. Wrong passwords
  answer -2203 (cameras -40401), a locked device -2101, a request under a
  stale sequence number or of an unknown session -2402 in clear.
- `tplinksim -tpap` also serves a P110M plug on TPAP.
- set_device_info is answered without a result (an empty one failed the
  tapo crate's client).
- Interop: the tapo crate's Python bindings 0.11.1 log in over TPAP to the
  simulated plug and camera (image tag now
  `sigma-tplink-sim-tapo-interop:tapo-0.11.1-r2`).

## v0.3.0 (2026-10-11)

Additions only; the v0.2.0 API is unchanged. Requires tplink v0.3.0.

- Tapo schedule and countdown writes: add_schedule_rule (ids S1, S2, ...;
  full and same-time refusals), edit_schedule_rule, remove_schedule_rules
  (some or all), add_countdown_rule (one at a time), remove_countdown_rules.
- Cameras (`CamState`, part of `TapoState`): the motor (motorMove,
  relativeMove, manualCalibrate, the single "do" form; `CamPosition`),
  saved positions (addMotorPostion, motorMoveToPreset, deletePreset), the
  siren answered without the module level as recorded devices do, the
  manual alarm (`CamAlarm`), recordings (`Recordings`, `CamClip`;
  searchDateWithVideo, searchVideoWithUTC), rebootDevice, formatSdCard
  and scanApList (`CamCounts`); the time zone kept once asked, so
  setTimezone changes it.
- Robot vacuums (`VacuumState`, `NewVacuumState`, `StartVacuum`,
  `VacuumMap`, `VacuumRoom`, `VacuumComponents`): status, cleaning by
  home or rooms, pause, return to dock, settings, consumables, records,
  maps and rooms, mop, dock emptying, volume, carpet; `Advance` moves a
  clean on.
- Interop tests: python-kasa 0.11.0.1's camera modules and vacuum
  modules against the simulators; the tapo crate's Python bindings 0.11.1
  writing schedules and a countdown, in a second image
  (`sigma-tplink-sim-tapo-interop:tapo-0.11.1`); the python-kasa image is
  now tagged `sigma-tplink-sim-interop:python-kasa-0.11.0.1-r3`.

## v0.2.0 (2026-10-11)

Additions only; the v0.1.0 API is unchanged. Requires tplink v0.2.0.

- Kasa modules beyond switching (`KasaState`): energy day and month
  statistics and erasing them, runtime statistics, schedule, countdown
  and away-mode rules, dimmer transitions, fades, ramp rate and
  parameters, light state, details and presets, light strip effects,
  LED, name (also per outlet), reboot, cloud information and firmware
  list, Wi-Fi scan and join; bulb module names answered too. Accessors
  `Rules`, `Joined`, `Reboots`, `DimmerSetting`; demo figures
  `DemoDayWh`, `DemoDayMinutes`.
- Tapo modules beyond switching (`TapoState`): extra components,
  settings kept per name, rule lists, presets, light effects (also edited
  by python-kasa), energy readings, gains and hourly, daily and monthly
  history (`DemoHourWh`), device use, a firmware update in steps, Wi-Fi
  scan, set-up with a new account, reboot; children answer
  multipleRequest, component_nego, the time, cloud state and sensor
  records; get_child_device_component_list. Accessors `Setting`,
  `QuickSetup`, `TapoReboots`.
- New devices: KLAP over HTTPS (`StartTapoHTTPS`), Tapo cameras and the
  H200 hub on the HTTPS transport (`StartCamera`, with `CamSections`,
  `PasswordMD5`, `PlainLogin`, `TagFailures`, `ForgetCameraSessions`),
  Kasa cameras (`StartKasaCamera`, `AnyLogin`), robot vacuums on the plain
  HTTPS transport (`StartTapoSSL`).
- The 20002 responder sends the answer's encrypted part on demand
  (`DiscoverySecret`) and describes HTTPS and camera devices.
- `NewDemo` and `tplinksim` also serve an HS220 dimmer, a KL430 light
  strip, an H100 hub with three sensors and a C210 camera
  (`DemoCameraUser`, `DemoCameraPassword`).
- `Kasa.Do` and `Tapo.Do` change a serving device's fields under its
  lock.
- Fixed: the 20002 answer read a device's information while a request
  changed it (a data race).
- Interop tests extended to every addition with python-kasa 0.11.0.1's
  client (python:3.12.10-slim); the peer image is now tagged
  `sigma-tplink-sim-interop:python-kasa-0.11.0.1-r2`.

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
