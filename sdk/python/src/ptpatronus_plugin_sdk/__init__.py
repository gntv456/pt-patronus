# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors

"""PTPatronus Plugin SDK (Python) — runtime client + types for external-http plugins.

A thin, pure-stdlib client that mirrors the TypeScript SDK (`@ptpatronus/plugin-sdk`).
Import in your plugin process:

    from ptpatronus_plugin_sdk import PTPHostClient

    host = PTPHostClient.from_env()  # reads PTP_HOST_URL / PTP_HOST_TOKEN
    host.log("info", "myplugin.start", "started")
    await-less sync calls; see README.
"""
from __future__ import annotations

from .client import PTPHostClient, HostError
from .types import HostContext, SiteRef, KVResult, HostCapabilities, HostTool

__all__ = [
    "PTPHostClient",
    "HostError",
    "HostContext",
    "SiteRef",
    "KVResult",
    "HostCapabilities",
    "HostTool",
]

__version__ = "0.1.0"
