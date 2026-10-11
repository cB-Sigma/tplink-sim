# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# python-kasa (GPL-3.0, github.com/python-kasa/python-kasa) as an interop
# peer: its client. It is installed from PyPI at build time inside the
# image only; nothing of python-kasa is copied into this repository.
FROM python:3.12.10-slim
ARG KASA_VERSION=0.11.0.1
RUN pip install --no-cache-dir python-kasa==${KASA_VERSION}
COPY client.py /peer/client.py
WORKDIR /peer
