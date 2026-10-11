# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# python-kasa's client against simulated devices, for interop tests.
#
#   python client.py legacy <host> <port>
#   python client.py klap1 <host> <port> <user> <pass>
#   python client.py klap2 <host> <port> <user> <pass>
#   python client.py aes <host> <port> <user> <pass> <login version>
#   python client.py tdp <host> <port> <user> <pass>
#   python client.py tdp-secret <host> <port> <user> <pass>
#   python client.py kasa-plug|kasa-dimmer|kasa-light <host> <port>
#   python client.py tapo-plug|tapo-bulb|tapo-hub <host> <port> <user> <pass>
#   python client.py klap-https|camera|camera-hub|plain-https <host> <port> <user> <pass>
#   python client.py linkie <host> <port>
#
# Each step prints "PEER <step> <json>" (or "PEER <step>-error <json>");
# "PEER done" ends the run. The protocol steps use python-kasa's protocol
# and transport classes; the "device" steps its Device API.

import asyncio
import json
import sys
import traceback

from kasa import Credentials, Device, DeviceConfig, Discover, Module
from kasa.device_factory import get_protocol
from kasa.deviceconfig import DeviceConnectionParameters, DeviceEncryptionType, DeviceFamily


def say(step, obj=None):
    print("PEER " + step + ("" if obj is None else " " + json.dumps(obj, sort_keys=True, default=str)), flush=True)


async def step(name, coro):
    try:
        r = await coro
        say(name, r if r is not None else {})
        return r
    except Exception as e:
        say(name + "-error", {"error": f"{type(e).__name__}: {e}", "trace": traceback.format_exc()})
        return None


def config(host, port, family, enc, creds=None, lv=None, https=False):
    ct = DeviceConnectionParameters(DeviceFamily(family), DeviceEncryptionType(enc), login_version=lv, https=https, http_port=int(port) if enc != "XOR" else None)
    return DeviceConfig(host=host, port_override=int(port), credentials=creds, connection_type=ct, timeout=5)


async def device_steps(cfg, switch):
    dev = await step("connect", _connect(cfg))
    if dev is None:
        return
    say("device", {"model": dev.model, "alias": dev.alias, "is_on": dev.is_on, "mac": dev.mac})
    await step("switch", getattr(dev, switch)())
    await step("update", dev.update())
    say("device2", {"is_on": dev.is_on})
    await dev.disconnect()


async def _connect(cfg):
    return await Device.connect(config=cfg)


async def legacy(host, port):
    cfg = config(host, port, "IOT.SMARTPLUGSWITCH", "XOR")
    proto = get_protocol(cfg)
    await step("query", proto.query({"system": {"get_sysinfo": {}}}))
    await step("relay", proto.query({"system": {"set_relay_state": {"state": 0}}}))
    await proto.close()
    await device_steps(config(host, port, "IOT.SMARTPLUGSWITCH", "XOR"), "turn_on")
    found = await step("discover-raw", _discover(host, port=int(port)))
    say("done")


async def _discover(host, **kw):
    devs = await Discover.discover(target=host, discovery_timeout=3, **kw)
    out = {}
    for ip, d in devs.items():
        out[ip] = {"model": d.model, "type": str(d.config.connection_type.device_family.value), "encrypt": str(d.config.connection_type.encryption_type.value),
                   "lv": d.config.connection_type.login_version, "http_port": d.config.connection_type.http_port}
        try:
            await d.update()
            out[ip]["is_on"] = d.is_on
        except Exception as e:
            out[ip]["update_error"] = f"{type(e).__name__}: {e}"
        await d.disconnect()
    return out


async def smart(host, port, user, pw, family, enc, lv=None):
    creds = Credentials(user, pw)
    cfg = config(host, port, family, enc, creds, lv)
    proto = get_protocol(cfg)
    await step("query", proto.query("get_device_info"))
    await step("set", proto.query({"set_device_info": {"device_on": True}}))
    await step("query2", proto.query("get_device_info"))
    await proto.close()
    await device_steps(config(host, port, family, enc, creds, lv), "turn_off")
    say("done")


async def klap1(host, port, user, pw):
    creds = Credentials(user, pw)
    cfg = config(host, port, "IOT.SMARTPLUGSWITCH", "KLAP", creds)
    proto = get_protocol(cfg)
    await step("query", proto.query({"system": {"get_sysinfo": {}}}))
    await step("relay", proto.query({"system": {"set_relay_state": {"state": 0}}}))
    await proto.close()
    await device_steps(config(host, port, "IOT.SMARTPLUGSWITCH", "KLAP", creds), "turn_on")
    say("done")


async def tdp(host, port, user, pw):
    Discover.DISCOVERY_PORT_2 = int(port)
    await step("discover", _discover(host, credentials=Credentials(user, pw)))
    say("done")


# --- Modules beyond switching (v0.2.0) ---------------------------------------

def rules(mod):
    return [r.to_dict() for r in mod.rules]


async def call(name, fn):
    """A step that runs a synchronous reader."""
    async def run():
        return fn()
    return await step(name, run())


async def kasa_plug(host, port):
    """Energy statistics, runtime, rules, LED, name, Wi-Fi and reboot of a
    legacy plug, through the Device API."""
    dev = await step("connect", _connect(config(host, port, "IOT.SMARTPLUGSWITCH", "XOR")))
    if dev is None:
        say("done")
        return
    e = dev.modules[Module.Energy]
    say("energy", {"power": e.current_consumption, "today": e.consumption_today, "month": e.consumption_this_month, "voltage": e.voltage})
    await step("daily", e.get_daily_stats(kwh=False))
    await step("monthly", e.get_monthly_stats(kwh=False))
    await step("usage-daily", dev.modules[Module.IotUsage].get_daystat())
    sched = dev.modules[Module.IotSchedule]
    await call("schedule", lambda: rules(sched))
    await call("antitheft", lambda: rules(dev.modules[Module.IotAntitheft]))
    if sched.rules:
        await step("delete-rule", sched.delete_rule(sched.rules[0]))
    await step("led", dev.modules[Module.Led].set_led(False))
    await step("alias", dev.set_alias("Renamed by python-kasa"))
    await step("erase", e.erase_stats())
    await step("scan", _scan(dev))
    await step("join", dev.wifi_join("Demo-Home", "wifi-password"))
    await step("reboot", dev.reboot())
    await dev.disconnect()
    say("done")


async def _scan(dev):
    return [{"ssid": n.ssid, "key_type": n.key_type} for n in await dev.wifi_scan()]


async def kasa_dimmer(host, port):
    dev = await step("connect", _connect(config(host, port, "IOT.SMARTPLUGSWITCH", "XOR")))
    if dev is None:
        say("done")
        return
    say("device", {"type": str(dev.device_type), "brightness": dev.modules[Module.Light].brightness})
    await step("transition", dev.set_dimmer_transition(70, 1500))
    from datetime import timedelta
    await step("fade", dev.modules[Module.IotDimmer].set_fade_on_time(timedelta(milliseconds=2500)))
    await step("update", dev.update())
    say("device2", {"brightness": dev.modules[Module.Light].brightness, "is_on": dev.is_on})
    await dev.disconnect()
    say("done")


async def kasa_light(host, port):
    dev = await step("connect", _connect(config(host, port, "IOT.SMARTBULB", "XOR")))
    if dev is None:
        say("done")
        return
    light = dev.modules[Module.Light]
    say("device", {"type": str(dev.device_type), "hsv": list(light.hsv) if light.has_feature("hsv") else None})
    await step("hsv", light.set_hsv(240, 100, 60, transition=1000))
    if Module.LightEffect in dev.modules:
        fx = dev.modules[Module.LightEffect]
        say("effects", {"list": fx.effect_list[:3]})
        await step("effect", fx.set_effect(fx.effect_list[1]))
    await step("update", dev.update())
    say("device2", {"hsv": list(light.hsv), "effect": dev.modules[Module.LightEffect].effect if Module.LightEffect in dev.modules else None})
    await dev.disconnect()
    say("done")


async def tapo_plug(host, port, user, pw):
    """Energy, firmware update, auto-off, LED, Wi-Fi and reboot of a Tapo
    plug, through the Device API."""
    dev = await step("connect", _connect(config(host, port, "SMART.TAPOPLUG", "KLAP", Credentials(user, pw), 2)))
    if dev is None:
        say("done")
        return
    say("modules", sorted(str(m) for m in dev.modules))
    e = dev.modules[Module.Energy]
    say("energy", {"power": e.current_consumption, "today": e.consumption_today, "month": e.consumption_this_month, "voltage": e.voltage})
    fw = dev.modules[Module.Firmware]
    info = await step("latest", fw.check_latest_firmware())
    say("latest-info", {"available": fw.update_available, "version": fw.latest_firmware})
    await step("update-fw", fw.update())
    ao = dev.modules[Module.AutoOff]
    await step("auto-off", ao.set_delay(45))
    await step("update-ao", dev.update())  # python-kasa sends both fields from its last update
    await step("auto-off-on", ao.set_enabled(True))
    await step("led", dev.modules[Module.Led].set_led(False))
    await step("scan", _scan(dev))
    await step("reboot", dev.reboot())
    await step("update", dev.update())
    say("device2", {"fw": dev.hw_info.get("sw_ver"), "auto_off": ao.enabled, "delay": ao.delay, "led": dev.modules[Module.Led].led})
    await dev.disconnect()
    say("done")


async def tapo_bulb(host, port, user, pw):
    dev = await step("connect", _connect(config(host, port, "SMART.TAPOBULB", "KLAP", Credentials(user, pw), 2)))
    if dev is None:
        say("done")
        return
    say("modules", sorted(str(m) for m in dev.modules))
    light = dev.modules[Module.Light]
    await step("hsv", light.set_hsv(120, 80, 40))
    tr = dev.modules[Module.LightTransition]
    await step("transition", tr.set_turn_on_transition(4))
    fx = dev.modules[Module.LightEffect]
    say("effects", {"list": fx.effect_list})
    await step("effect", fx.set_effect(fx.effect_list[1]))
    pr = dev.modules[Module.LightPreset]
    say("presets", {"list": pr.preset_list})
    await step("update", dev.update())
    say("device2", {"hsv": list(light.hsv), "on_transition": tr.turn_on_transition, "effect": fx.effect})
    await dev.disconnect()
    say("done")


async def tapo_hub(host, port, user, pw):
    dev = await step("connect", _connect(config(host, port, "SMART.TAPOHUB", "KLAP", Credentials(user, pw), 2)))
    if dev is None:
        say("done")
        return
    out = {}
    for c in dev.children:
        f = {k: v.value for k, v in c.features.items() if k in ("temperature", "humidity", "battery_low", "is_open", "battery_level")}
        out[c.device_id] = {"model": c.model, "alias": c.alias, "features": f, "modules": sorted(str(m) for m in c.modules)}
        if Module.TriggerLogs in c.modules:
            out[c.device_id]["logs"] = [l.event for l in c.modules[Module.TriggerLogs].logs]
    say("children", out)
    await dev.disconnect()
    say("done")


async def klap_https(host, port, user, pw):
    """KLAP v2 over HTTPS, through the protocol and the Device API."""
    creds = Credentials(user, pw)
    proto = get_protocol(config(host, port, "SMART.TAPOPLUG", "KLAP", creds, 2, https=True))
    await step("query", proto.query("get_device_info"))
    await proto.close()
    await device_steps(config(host, port, "SMART.TAPOPLUG", "KLAP", creds, 2, https=True), "turn_off")
    say("done")


async def camera(host, port, user, pw, family):
    """A camera or H200 hub on the HTTPS transport, through the Device
    API."""
    dev = await step("connect", _connect(config(host, port, family, "AES", Credentials(user, pw), 2, https=True)))
    if dev is None:
        say("done")
        return
    say("device", {"model": dev.model, "alias": dev.alias, "type": str(dev.device_type), "fw": dev.hw_info.get("sw_ver"),
                   "children": sorted(c.device_id for c in dev.children)})
    await step("alias", dev.set_alias("Renamed by python-kasa"))
    await step("update", dev.update())
    say("device2", {"alias": dev.alias})
    if dev.children:
        c = dev.children[0]
        f = {k: v.value for k, v in c.features.items() if k in ("temperature", "humidity")}
        say("child", {"model": c.model, "features": f})
    await dev.disconnect()
    say("done")


async def linkie(host, port):
    """A Kasa camera on its HTTPS transport (python-kasa sends its own
    default login)."""
    proto = get_protocol(config(host, port, "IOT.IPCAMERA", "XOR"))
    await step("query", proto.query({"system": {"get_sysinfo": {}}}))
    await step("alias", proto.query({"system": {"set_dev_alias": {"alias": "Renamed by python-kasa"}}}))
    await proto.close()
    say("done")


async def plain_https(host, port, user, pw):
    """A robot vacuum on the plain HTTPS transport."""
    proto = get_protocol(config(host, port, "SMART.TAPOROBOVAC", "AES", Credentials(user, pw), 2))
    say("transport", type(proto._transport).__name__)
    await step("query", proto.query("get_device_info"))
    await step("multi", proto.query({"get_device_info": None, "get_device_time": None}))
    await proto.close()
    say("done")


async def tdp_secret(host, port, user, pw):
    """The decrypted part of the 20002 answer, as python-kasa reads it."""
    Discover.DISCOVERY_PORT_2 = int(port)

    async def run():
        devs = await Discover.discover(target=host, discovery_timeout=3, credentials=Credentials(user, pw))
        out = {}
        for ip, d in devs.items():
            out[ip] = (d._discovery_info or {}).get("decrypted_data")
            await d.disconnect()
        return out
    await step("discover", run())
    say("done")


async def main():
    try:
        await run_mode()
    except Exception as e:
        say("fatal-error", {"error": f"{type(e).__name__}: {e}", "trace": traceback.format_exc()})
        say("done")


async def run_mode():
    mode, args = sys.argv[1], sys.argv[2:]
    if mode == "legacy":
        await legacy(*args)
    elif mode == "klap1":
        await klap1(*args)
    elif mode == "klap2":
        await smart(*args, "SMART.TAPOPLUG", "KLAP", 2)
    elif mode == "aes":
        await smart(*args[:4], "SMART.TAPOPLUG", "AES", int(args[4]))
    elif mode == "tdp":
        await tdp(*args)
    elif mode == "tdp-secret":
        await tdp_secret(*args)
    elif mode == "klap-https":
        await klap_https(*args)
    elif mode == "linkie":
        await linkie(*args)
    elif mode == "plain-https":
        await plain_https(*args)
    elif mode == "camera":
        await camera(*args, "SMART.IPCAMERA")
    elif mode == "camera-hub":
        await camera(*args, "SMART.TAPOHUB")
    elif mode == "kasa-plug":
        await kasa_plug(*args)
    elif mode == "kasa-dimmer":
        await kasa_dimmer(*args)
    elif mode == "kasa-light":
        await kasa_light(*args)
    elif mode == "tapo-plug":
        await tapo_plug(*args)
    elif mode == "tapo-bulb":
        await tapo_bulb(*args)
    elif mode == "tapo-hub":
        await tapo_hub(*args)


asyncio.run(main())
