import json
import http.client
from typing import Any, Dict, Optional


Json = Dict[str, Any]


class MCPSSEClient:
    """
    Minimal SSE-based MCP client for the 2024-11-05 transport.

    - GET / to open SSE stream; first event is 'endpoint' with a messages path
    - POST JSON-RPC messages to that endpoint
    - Responses arrive as SSE 'message' events containing a JSON object/array
    """

    def __init__(self, host: str, port: int, protocol_version: str = "2024-11-05"):
        self.host = host
        self.port = port
        self.protocol_version = protocol_version
        self.conn = http.client.HTTPConnection(host, port, timeout=30)
        self._next_id = 1
        self._endpoint_path: Optional[str] = None
        self._open_sse()

    def _open_sse(self) -> None:
        self.conn.request("GET", "/", headers={"Accept": "text/event-stream"})
        resp = self.conn.getresponse()
        if resp.status != 200 or (resp.getheader("Content-Type") or "").split(";")[0].strip() != "text/event-stream":
            raise RuntimeError(f"SSE GET failed: {resp.status} {resp.reason}")
        # Parse events until we receive 'endpoint'
        endpoint: Optional[str] = None
        data_buf: bytes = b""
        event_type: Optional[str] = None
        while True:
            line = resp.readline()
            if not line:
                raise RuntimeError("SSE stream ended before endpoint event")
            line = line.rstrip(b"\r\n")
            if line.startswith(b"event:"):
                event_type = line[6:].lstrip().decode("utf-8")
            elif line.startswith(b"data:"):
                data_buf += line[5:].lstrip() + b"\n"
            elif line == b"":
                if event_type == "endpoint":
                    endpoint = data_buf.decode("utf-8").strip()
                    break
                # reset buffer for any other event
                event_type = None
                data_buf = b""
        # The endpoint is a relative URI, e.g. '?sessionid=abc'
        if not endpoint:
            raise RuntimeError("No endpoint provided by server")
        self._endpoint_path = endpoint
        # Keep the GET open; http.client will keep the response socket bound to this connection.
        # We'll create separate connections for POSTs to avoid interfering with the GET.
        self._sse_response = resp

    def _post(self, payload: Json):
        # Use a separate connection to avoid blocking the GET reader on the same http.client
        conn = http.client.HTTPConnection(self.host, self.port, timeout=30)
        body = json.dumps(payload).encode("utf-8")
        conn.request("POST", self._endpoint_path or "/", body=body, headers={"Content-Type": "application/json"})
        resp = conn.getresponse()
        # The SSE server responds 202 Accepted for POSTs
        resp.read()  # drain
        conn.close()
        if resp.status not in (200, 202, 204):
            raise RuntimeError(f"SSE POST failed: {resp.status} {resp.reason}")

    def _await_response(self, req_id: int, timeout: float = 30.0) -> Json:
        # Read SSE stream until we see a data event containing a JSON-RPC message
        # with the matching id.
        import time

        start = time.time()
        buf = b""
        while time.time() - start < timeout:
            line = self._sse_response.readline()
            if not line:
                break
            line = line.rstrip(b"\r\n")
            if line.startswith(b"data:"):
                buf += line[5:].lstrip() + b"\n"
            elif line == b"":
                if buf:
                    try:
                        msg = json.loads(buf.decode("utf-8"))
                    except Exception:
                        msg = None
                    buf = b""
                    if isinstance(msg, dict):
                        if msg.get("id") == req_id:
                            return msg
                    elif isinstance(msg, list):
                        for m in msg:
                            if isinstance(m, dict) and m.get("id") == req_id:
                                return m
        raise TimeoutError(f"Timed out waiting for SSE response to id {req_id}")

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
        self._post(req)
        return self._await_response(rid)

    def send_initialized(self) -> None:
        note = {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}
        self._post(note)

    def request(self, method: str, params: Optional[Dict[str, Any]] = None) -> Json:
        rid = self._next_id
        self._next_id += 1
        req: Json = {"jsonrpc": "2.0", "id": rid, "method": method}
        if params is not None:
            req["params"] = params
        self._post(req)
        return self._await_response(rid)

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
        txt = MCPSSEClient.parse_text_content(result_obj)
        return json.loads(txt) if txt else None

