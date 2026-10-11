# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# The tapo package's client against a simulated device in TPAP mode, found
# by its address: tapo asks the device whether it speaks TPAP, logs in and
# sends requests.
#
#   python tpap.py plug <host:port> <user> <pass>
#   python tpap.py camera <host:port> <user> <pass>
#   python tpap.py wrong <host:port> <user> <pass>
#
# Each step prints "PEER <step> <json>" (or "PEER <step>-error <json>");
# "PEER done" ends the run.

import asyncio
import json
import sys
import traceback

from tapo import ApiClient


def say(step, obj=None):
    print("PEER " + step + ("" if obj is None else " " + json.dumps(obj, sort_keys=True, default=str)), flush=True)


async def step(name, coro):
    try:
        r = await coro
        say(name, r if r is None or isinstance(r, (dict, list, int, str, bool)) else repr(r))
        return r
    except Exception as e:
        say(name + "-error", {"error": f"{type(e).__name__}: {e}", "trace": traceback.format_exc()})
        return None


async def plug(addr, user, pw):
    p = await step("connect", ApiClient(user, pw, timeout_s=10).p110(addr))
    if p is None:
        return
    await step("info", p.get_device_info_json())
    await step("on", p.on())
    await step("info-on", p.get_device_info_json())
    await step("off", p.off())
    await step("info-off", p.get_device_info_json())
    usage = await step("usage", p.get_energy_usage())
    if usage is not None:
        say("usage-dict", usage.to_dict())


async def camera(addr, user, pw):
    c = await step("connect", ApiClient(user, pw, timeout_s=10).c220(addr))
    if c is None:
        return
    await step("info", c.get_device_info_json())


async def wrong(addr, user, pw):
    try:
        await ApiClient(user, pw + "-wrong", timeout_s=10).p110(addr)
        say("wrong-accepted")
    except Exception as e:
        say("wrong", {"error": f"{type(e).__name__}: {e}"})


async def main():
    mode, addr, user, pw = sys.argv[1:5]
    await {"plug": plug, "camera": camera, "wrong": wrong}[mode](addr, user, pw)
    say("done")


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except Exception as e:
        say("fatal-error", {"error": f"{type(e).__name__}: {e}", "trace": traceback.format_exc()})
        say("done")
