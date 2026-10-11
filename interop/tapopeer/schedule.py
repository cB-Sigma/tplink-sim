# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# The tapo package's client against a simulated plug: schedules and the
# countdown written, read back and removed.
#
#   python schedule.py <host:port> <user> <pass>
#
# Each step prints "PEER <step> <json>" (or "PEER <step>-error <json>");
# "PEER done" ends the run.

import asyncio
import json
import sys
import traceback

from tapo import ApiClient
from tapo.requests import DaysOfWeek, ScheduleRule
from tapo.responses import PowerState


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


def rule_dict(r):
    d = r.to_dict()
    return {k: d.get(k) for k in ("id", "enabled", "time", "days", "desired_state")}


async def main():
    addr, user, pw = sys.argv[1:4]
    plug = await step("connect", ApiClient(user, pw, timeout_s=10).p110(addr))
    if plug is None:
        say("done")
        return
    added = await step("add-weekly", plug.add_schedule_rule(
        ScheduleRule.clock_weekly(6, 30, DaysOfWeek.MON | DaysOfWeek.WED, PowerState.On)))
    if added is not None:
        say("added", rule_dict(added))
    await step("add-sunset", plug.add_schedule_rule(ScheduleRule.sunset_once(-15, PowerState.Off)))
    rules = await step("list", plug.get_schedule_rules())
    if rules is not None:
        say("rules", [rule_dict(r) for r in rules])
        await step("edit", plug.edit_schedule_rule(rules[0].to_editable().with_enabled(False)))
        await step("edit-time", plug.edit_schedule_rule(
            ScheduleRule.clock_weekly(7, 0, DaysOfWeek.MON | DaysOfWeek.WED, PowerState.Off).with_id(rules[0].id)))
    say("max", await plug.get_max_schedule_rules())
    timer = await step("timer", plug.set_timer(600, PowerState.On))
    if timer is not None:
        say("timer-set", timer.to_dict())
    got = await step("timer-get", plug.get_timer())
    if got is not None:
        say("timer-read", got.to_dict())
    rules = await step("list2", plug.get_schedule_rules())
    if rules is not None:
        say("rules2", [rule_dict(r) for r in rules])
    await step("clear-timer", plug.clear_timer())
    await step("remove-one", plug.remove_schedule_rule(rules[1].id if rules else "S0"))
    rules = await step("list3", plug.get_schedule_rules())
    if rules is not None:
        say("rules3", [rule_dict(r) for r in rules])
    # One rule is left for the Go client to read; then a rule the Go client
    # wrote is read here.
    say("ready-for-go")
    line = await asyncio.get_running_loop().run_in_executor(None, sys.stdin.readline)
    rules = await step("list4", plug.get_schedule_rules())
    if rules is not None:
        say("rules4", [rule_dict(r) for r in rules])
    await step("remove-all", plug.remove_all_schedule_rules())
    say("done")


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except Exception as e:
        say("fatal-error", {"error": f"{type(e).__name__}: {e}", "trace": traceback.format_exc()})
        say("done")
