"""Data types for the PTPatronus Plugin SDK (mirrors the TypeScript SDK surface)."""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional


@dataclass
class HostContext:
    protocol_version: int = 1
    plugin_id: str = ""
    base_url: str = ""
    token: str = ""
    permissions: List[str] = field(default_factory=list)
    host_capabilities: List[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict) -> "HostContext":
        data = data or {}
        return cls(
            protocol_version=int(data.get("protocol_version") or 1),
            plugin_id=str(data.get("plugin_id") or ""),
            base_url=str(data.get("base_url") or ""),
            token=str(data.get("token") or ""),
            permissions=list(data.get("permissions") or []),
            host_capabilities=list(data.get("host_capabilities") or []),
        )


@dataclass
class SiteRef:
    id: int
    name: str
    base_url: str
    username: Optional[str] = None


@dataclass
class KVResult:
    found: bool = False
    value: Any = None


@dataclass
class HostCapabilities:
    api_version: str = ""
    plugin_id: str = ""
    permissions: List[str] = field(default_factory=list)
    host_capabilities: List[str] = field(default_factory=list)
    endpoints: List[str] = field(default_factory=list)


@dataclass
class HostTool:
    """一个宿主工具的元数据（GET /tools 返回的单条）。"""
    name: str = ""
    description: str = ""
    input_schema: Dict[str, Any] = field(default_factory=dict)
    permission: str = ""        # 空=公开（无需权限）
    category: str = ""          # downloader/media/site/... 用于分组
    destructive: bool = False   # 破坏性（删数据/删文件）

    @classmethod
    def from_dict(cls, data: dict) -> "HostTool":
        data = data or {}
        return cls(
            name=str(data.get("name") or ""),
            description=str(data.get("description") or ""),
            input_schema=dict(data.get("input_schema") or {}),
            permission=str(data.get("permission") or ""),
            category=str(data.get("category") or ""),
            destructive=bool(data.get("destructive") or False),
        )
