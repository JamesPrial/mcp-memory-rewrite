import json
import os
import select
import subprocess
import threading
import time
from typing import Any, Dict, List, Optional, Tuple, Union


JsonMsg = Dict[str, Any]


class MCPStdioClient:
    """
    Minimal MCP client over stdio for tests.

    - Sends newline-delimited JSON-RPC 2.0 messages
    - Performs MCP initialize + notifications/initialized handshake
    - Provides helpers for tools/list and tools/call
    """

    def __init__(self, proc: subprocess.Popen):
        self.proc = proc
        assert proc.stdout is not None and proc.stdin is not None
        self._out = proc.stdout
        self._in = proc.stdin
        self._next_id = 1
        self._lock = threading.Lock()
        self._notifications: List[JsonMsg] = []

    def _write_msg(self, msg: JsonMsg) -> None:
        line = json.dumps(msg) + "\n"
        self._in.write(line)
        self._in.flush()

    def _read_msgs_once(self, timeout: float = 5.0) -> List[JsonMsg]:
        """Read one line (which may be a single message or array of messages)."""
        fd = self._out.fileno()
        rlist, _, _ = select.select([fd], [], [], timeout)
        if not rlist:
            raise TimeoutError("Timed out waiting for server output")
        line = self._out.readline()
        if line == "":
            raise RuntimeError("Server closed stdout (no more JSON)")
        data = json.loads(line)
        if isinstance(data, list):
            return data
        return [data]

    def _await_response(self, req_id: int, timeout: float = 10.0) -> JsonMsg:
        deadline = time.time() + timeout
        while time.time() < deadline:
            msgs = self._read_msgs_once(timeout=max(0.0, deadline - time.time()))
            for msg in msgs:
                # Notifications have no id
                if "id" not in msg:
                    self._notifications.append(msg)
                    continue
                if msg.get("id") == req_id:
                    return msg
            # keep looping until matching id arrives
        raise TimeoutError(f"No response for id {req_id} within {timeout}s")

    def initialize(self, client_name: str = "e2e-tests", version: str = "0.0.1",
                   protocol_version: str = "2025-06-18") -> JsonMsg:
        with self._lock:
            rid = self._next_id
            self._next_id += 1
        req = {
            "jsonrpc": "2.0",
            "id": rid,
            "method": "initialize",
            "params": {
                "protocolVersion": protocol_version,
                "clientInfo": {"name": client_name, "version": version},
                "capabilities": {},
            },
        }
        self._write_msg(req)
        resp = self._await_response(rid)
        if "error" in resp:
            raise RuntimeError(f"initialize failed: {resp['error']}")
        return resp

    def send_initialized(self) -> None:
        note = {
            "jsonrpc": "2.0",
            "method": "notifications/initialized",
            "params": {},
        }
        self._write_msg(note)

    def request(self, method: str, params: Optional[Dict[str, Any]] = None,
                timeout: float = 30.0) -> JsonMsg:
        with self._lock:
            rid = self._next_id
            self._next_id += 1
        req: JsonMsg = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            req["params"] = params
        self._write_msg(req)
        resp = self._await_response(rid, timeout=timeout)
        return resp

    def tools_list(self) -> JsonMsg:
        return self.request("tools/list", {})

    def tools_call(self, name: str, arguments: Optional[Dict[str, Any]] = None,
                   timeout: float = 30.0) -> JsonMsg:
        params = {"name": name}
        if arguments is not None:
            params["arguments"] = arguments
        return self.request("tools/call", params, timeout=timeout)

    @staticmethod
    def parse_text_content(result_obj: Dict[str, Any]) -> str:
        """Extract first text content from a CallToolResult wire object."""
        content = result_obj.get("result", {}).get("content")
        if not isinstance(content, list):
            raise AssertionError("result.content missing or not a list")
        for item in content:
            if isinstance(item, dict) and item.get("type") == "text":
                return item.get("text", "")
        raise AssertionError("No text content found in result.content")

    @staticmethod
    def parse_text_json(result_obj: Dict[str, Any]) -> Any:
        txt = MCPStdioClient.parse_text_content(result_obj)
        return json.loads(txt) if txt else None

    def close(self, timeout: float = 5.0) -> None:
        try:
            if self.proc.stdin:
                try:
                    self.proc.stdin.close()
                except Exception:
                    pass
            if self.proc.stdout:
                # Give the process a chance to exit cleanly
                start = time.time()
                while time.time() - start < timeout and self.proc.poll() is None:
                    time.sleep(0.05)
        finally:
            if self.proc.poll() is None:
                try:
                    self.proc.terminate()
                except Exception:
                    pass
                try:
                    self.proc.wait(timeout=timeout)
                except Exception:
                    try:
                        self.proc.kill()
                    except Exception:
                        pass

