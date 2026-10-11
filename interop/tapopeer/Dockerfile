# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# The Python bindings of the tapo Rust crate (MIT,
# github.com/mihai-dinculescu/tapo) as a second interop peer: its client
# writes Tapo schedules and countdowns, which python-kasa does not. It is
# installed from PyPI at build time inside the image only; nothing of it is
# copied into this repository.
FROM python:3.12.10-slim
ARG TAPO_VERSION=0.11.1
RUN pip install --no-cache-dir tapo==${TAPO_VERSION}
COPY schedule.py /peer/schedule.py
WORKDIR /peer
