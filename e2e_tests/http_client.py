import json
import http.client
import time
from typing import Any, Dict, Optional


Json = Dict[str, Any]


class MCPHTTPStreamableClient:
    """
    Minimal Streamable HTTP MCP client using stdlib http.client.

    - Sends POSTs with Accept: application/json, text/event-stream
    - Parses JSON or SSE response and returns the JSON-RPC message
    - Tracks Mcp-Session-Id and sets Mcp-Protocol-Version
    """

    def __init__(self, host: str, port: int, protocol_version: str = "2025-06-18"):
        self.host = host
        self.port = port
        self.protocol_version = protocol_version
        self.session_id: str = ""
        self._next_id = 1

    def _headers(self) -> Dict[str, str]:
        h = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "Mcp-Protocol-Version": self.protocol_version,
            # Avoid persistent connections to simplify server behavior in tests
            "Connection": "close",
        }
        if self.session_id:
            h["Mcp-Session-Id"] = self.session_id
        return h

    def _send_and_read(self, payload: Json, expect_response: bool = True) -> Optional[Json]:
        body = json.dumps(payload)
        conn = http.client.HTTPConnection(self.host, self.port, timeout=30)
        conn.request("POST", "/", body=body.encode("utf-8"), headers=self._headers())
        
        try:
            resp = conn.getresponse()
        except http.client.BadStatusLine as e:
            # Handle case where server closes connection immediately (e.g., for notifications)
            # BadStatusLine with message "0" means the server closed without sending a response
            if not expect_response and str(e).strip() in ("0", ""):
                conn.close()
                return None
            raise

        # capture session id if provided
        sid = resp.getheader("Mcp-Session-Id")
        if sid and not self.session_id:
            self.session_id = sid
        elif sid and self.session_id and sid != self.session_id:
            resp.read()
            raise RuntimeError(f"Session ID changed: {self.session_id} -> {sid}")

        if resp.status in (202, 204) or not expect_response:
            try:
                resp.read()  # drain
            finally:
                resp.close()
                conn.close()
            return None

        ctype = (resp.getheader("Content-Type") or "").split(";")[0].strip()
        if ctype == "application/json":
            data = resp.read()
            try:
                return json.loads(data) if data else None
            finally:
                resp.close()
                conn.close()
        elif ctype == "text/event-stream":
            # parse SSE events until we get a JSON-RPC message
            buf = b""
            target_id = payload.get("id")
            while True:
                chunk = resp.readline()
                if not chunk:
                    break
                line = chunk.rstrip(b"\r\n")
                if line.startswith(b"data:"):
                    # accumulate data: lines (strip 'data: ')
                    buf += line[5:].lstrip() + b"\n"
                elif line == b"":
                    # end of event
                    if buf:
                        try:
                            msg = json.loads(buf.decode("utf-8"))
                        except Exception:
                            msg = None
                        buf = b""
                        if isinstance(msg, list):
                            for m in msg:
                                if not expect_response:
                                    continue
                                if m.get("id") == target_id:
                                    try:
                                        return m
                                    finally:
                                        resp.close()
                                        conn.close()
                        elif isinstance(msg, dict):
                            if not expect_response:
                                continue
                            if msg.get("id") == target_id:
                                try:
                                    return msg
                                finally:
                                    resp.close()
                                    conn.close()
                # ignore other event fields (id:, event:, etc.)
            # if we exit without returning, drain
            resp.close()
            conn.close()
            return None
        else:
            data = resp.read()
            try:
                raise RuntimeError(f"Unexpected content-type {ctype}, status={resp.status}, body={data[:200]!r}")
            finally:
                resp.close()
                conn.close()

    def initialize(self, client_name: str = "e2e-tests", version: str = "0.0.1") -> Json:
        rid = self._next_id
        self._next_id += 1
        req = {
            "jsonrpc": "2.0",
            "id": rid,
            "method": "initialize",
            "params": {
                "protocolVersion": self.protocol_version,
                "clientInfo": {"name": client_name, "version": version},
                "capabilities": {},
            },
        }
        resp = self._send_and_read(req)
        if not resp or "error" in resp:
            raise RuntimeError(f"initialize failed: {resp}")
        return resp

    def send_initialized(self) -> None:
        note = {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}
        self._send_and_read(note, expect_response=False)

    def request(self, method: str, params: Optional[Dict[str, Any]] = None) -> Json:
        rid = self._next_id
        self._next_id += 1
        req: Json = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            req["params"] = params
        return self._send_and_read(req) or {}

    def tools_list(self) -> Json:
        return self.request("tools/list", {})

    def tools_call(self, name: str, arguments: Optional[Dict[str, Any]] = None) -> Json:
        params: Dict[str, Any] = {"name": name}
        if arguments is not None:
            params["arguments"] = arguments
        return self.request("tools/call", params)

    @staticmethod
    def parse_text_content(result_obj: Dict[str, Any]) -> str:
        content = result_obj.get("result", {}).get("content")
        if not isinstance(content, list):
            raise AssertionError("result.content missing or not list")
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                return item.get("text", "")
        raise AssertionError("No text content in result.content")

    @staticmethod
    def parse_text_json(result_obj: Dict[str, Any]) -> Any:
        txt = MCPHTTPStreamableClient.parse_text_content(result_obj)
        return json.loads(txt) if txt else None
