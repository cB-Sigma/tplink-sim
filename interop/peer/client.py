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
#
# Each step prints "PEER <step> <json>" (or "PEER <step>-error <json>");
# "PEER done" ends the run. The protocol steps use python-kasa's protocol
# and transport classes; the "device" steps its Device API.

import asyncio
import json
import sys
import traceback

from kasa import Credentials, Device, DeviceConfig, Discover
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


def config(host, port, family, enc, creds=None, lv=None):
    ct = DeviceConnectionParameters(DeviceFamily(family), DeviceEncryptionType(enc), login_version=lv, http_port=int(port) if enc != "XOR" else None)
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


async def main():
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


asyncio.run(main())
