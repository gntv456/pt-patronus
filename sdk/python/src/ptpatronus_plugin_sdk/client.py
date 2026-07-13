"""PTPHostClient — synchronous Host API client for external-http plugins (pure stdlib).

Mirrors `@ptpatronus/plugin-sdk`'s PTPHostClient. All calls are synchronous (urllib); raise
`HostError` on non-2xx responses. The host gates by permission — calls requiring a permission
the plugin didn't declare return 403 (surfaced as HostError).
"""
from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional

from .types import HostCapabilities, HostContext, KVResult, SiteRef


class HostError(RuntimeError):
    """Raised when a Host API call fails (non-2xx or transport error)."""


class PTPHostClient:
    def __init__(self, context: Any):
        ctx = context if isinstance(context, HostContext) else HostContext.from_dict(context or {})
        if not ctx.base_url:
            raise ValueError("host base_url is required")
        if not ctx.token:
            raise ValueError("host token is required")
        self.context = ctx

    # ---- constructors ----

    @classmethod
    def from_env(cls, env: Optional[Dict[str, str]] = None) -> Optional["PTPHostClient"]:
        env = env if env is not None else os.environ
        base_url = (env.get("PTP_HOST_URL") or "").strip()
        token = env.get("PTP_HOST_TOKEN") or ""
        if not base_url or not token:
            return None
        caps = [c.strip() for c in (env.get("PTP_HOST_CAPABILITIES") or "").split(",") if c.strip()]
        return cls(HostContext(
            protocol_version=int(env.get("PTP_HOST_PROTOCOL") or 1),
            plugin_id=env.get("PTP_PLUGIN_ID") or "",
            base_url=base_url,
            token=token,
            host_capabilities=caps,
        ))

    @classmethod
    def from_payload(cls, payload: Dict[str, Any]) -> Optional["PTPHostClient"]:
        host = (payload or {}).get("host")
        if not host:
            return None
        return cls(HostContext.from_dict(host))

    # ---- permission / capability checks ----

    def has_permission(self, permission: str) -> bool:
        return _has_scoped_permission(self.context.permissions, permission)

    def has_capability(self, capability: str) -> bool:
        return capability in self.context.host_capabilities

    # ---- Host API ----

    def capabilities(self) -> HostCapabilities:
        data = self._request("GET", "/capabilities")
        return HostCapabilities(
            api_version=str(data.get("api_version") or ""),
            plugin_id=str(data.get("plugin_id") or ""),
            permissions=list(data.get("permissions") or []),
            host_capabilities=list(data.get("host_capabilities") or []),
            endpoints=list(data.get("endpoints") or []),
        )

    def config(self) -> Dict[str, Any]:
        return self._request("GET", "/config").get("config") or {}

    def set_runtime_config(self, key: str, value: Any) -> None:
        self._request("POST", "/runtime/config", {"key": key, "value": value})

    def log(self, level: str, event: str, message: str, data: Optional[Dict[str, Any]] = None) -> None:
        self._request("POST", "/log", {"level": level, "event": event, "message": message, "data": data or {}})

    def notice(self, title: str, body: str, level: str = "info") -> None:
        self._request("POST", "/notice", {"title": title, "body": body, "level": level})

    def publish(self, event_type: str, data: Optional[Dict[str, Any]] = None) -> None:
        self._request("POST", "/events", {"type": event_type, "data": data or {}})

    def kv_get(self, key: str) -> KVResult:
        data = self._request("GET", f"/kv/{urllib.parse.quote(key, safe='')}")
        return KVResult(found=bool(data.get("found")), value=data.get("value"))

    def kv_set(self, key: str, value: Any) -> None:
        self._request("PUT", f"/kv/{urllib.parse.quote(key, safe='')}", {"value": value})

    def kv_delete(self, key: str) -> None:
        self._request("DELETE", f"/kv/{urllib.parse.quote(key, safe='')}")

    def sites(self) -> List[SiteRef]:
        data = self._request("GET", "/sites")
        out: List[SiteRef] = []
        for s in data.get("sites") or []:
            out.append(SiteRef(
                id=int(s.get("id") or 0),
                name=str(s.get("name") or ""),
                base_url=str(s.get("base_url") or ""),
                username=s.get("username"),
            ))
        return out

    def site_cookie(self, site_id: int) -> str:
        return str(self._request("GET", f"/sites/{int(site_id)}/cookie").get("cookie") or "")

    def media_search(self, keyword: str, year: int = 0) -> List[Dict[str, Any]]:
        """Search the host media chain (TMDB/豆瓣/寸光集). Requires the `media:search` permission."""
        data = self._request("POST", "/media/search", {"keyword": keyword, "year": int(year or 0)})
        return list(data.get("results") or [])

    # ---- Tool RPC（统一能力入口；~80 个工具覆盖下载器/媒体服务器/订阅/云盘/刷流/RSS/字幕/辅种/整理/仪表盘/通知/过滤/文件/工作流）----

    def tools(self) -> List[Dict[str, Any]]:
        """List host tools this plugin is permitted to call (自描述：含 name/description/input_schema/permission)."""
        return list(self._request("GET", "/tools").get("tools") or [])

    def call(self, name: str, input: Optional[Dict[str, Any]] = None) -> Any:
        """Invoke a host tool by name. Returns the tool's raw JSON result (dict/list/scalar).

        权限由插件 manifest 的 permissions 声明控制（如 downloader:read、mediaserver:write）；
        权限不足时抛 HostError(HTTP 403)。可用工具见 ``tools()`` 或 DEVGUIDE 的完整目录。
        """
        return self._request("POST", f"/tools/{urllib.parse.quote(name, safe='')}", {"input": input or {}})

    # ---- 核心域便捷方法（薄封装，等价于 self.call(name, {...})；未列出的工具请用 call）----

    def torrents_list(self, downloader_id: int) -> Any:
        """List torrents in a downloader. Perm: downloader:read."""
        return self.call("torrents_list", {"downloader_id": int(downloader_id)})

    def torrents_add(self, downloader_id: int, url: str, category: str = "") -> Any:
        """Add a torrent by magnet/URL. Perm: downloader:write."""
        return self.call("torrents_add", {"downloader_id": int(downloader_id), "url": url, "category": category})

    def mediaserver_list(self) -> Any:
        """List media servers. Perm: mediaserver:read."""
        return self.call("mediaserver_list")

    def mediaserver_library_scan(self, server_id: int, library_id: str = "") -> Any:
        """Trigger a library scan (unblocks NFO/poster → refresh). Perm: mediaserver:write."""
        return self.call("mediaserver_library_scan", {"server_id": int(server_id), "library_id": library_id})

    def subscribe_list(self) -> Any:
        """List subscriptions. Perm: subscribe:read."""
        return self.call("subscribe_list")

    def send_notification(self, title: str, body: str, level: str = "info") -> Any:
        """Send a multi-channel notification (Bark/TG/QQ/WeChat/Webhook). Perm: notify:send.

        注意：``notice()`` 是站内收件箱；本方法是多通道外发（等价于宿主通知系统）。
        """
        return self.call("send_notification", {"title": title, "body": body, "level": level})

    def dashboard_overview(self) -> Any:
        """Aggregated cross-site overview. Perm: dashboard:read."""
        return self.call("dashboard_overview")

    # ---- internals ----

    def _request(self, method: str, path: str, body: Any = None) -> Dict[str, Any]:
        url = self.context.base_url.rstrip("/") + path
        data = None
        headers = {"Authorization": f"Bearer {self.context.token}", "Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=20) as res:
                raw = res.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            detail = ""
            try:
                detail = exc.read().decode("utf-8", "replace")
            except Exception:
                pass
            msg = detail
            try:
                msg = str((json.loads(detail) or {}).get("error") or detail)
            except Exception:
                pass
            raise HostError(f"Host API {method} {path} failed: HTTP {exc.code} {msg}".strip()) from exc
        except urllib.error.URLError as exc:
            raise HostError(f"Host API {method} {path} failed: {exc.reason}") from exc
        return json.loads(raw) if raw else {}


def _has_scoped_permission(permissions: List[str], permission: str) -> bool:
    for item in permissions:
        item = item.strip()
        if item in ("*", "host:*", "plugin:*", permission):
            return True
        if item.endswith(":*") and permission.startswith(item[:-2] + ":"):
            return True
    return False
