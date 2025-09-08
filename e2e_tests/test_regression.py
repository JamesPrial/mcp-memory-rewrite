import os
import socket
import subprocess
from pathlib import Path

import pytest

from .mcp_client import MCPStdioClient
from .http_client import MCPHTTPStreamableClient


REPO_ROOT = Path(__file__).resolve().parents[1]
BIN_DIR = REPO_ROOT / ".e2e_bin"
BIN_PATH = BIN_DIR / "mcp-memory-server"


def build_server_binary() -> Path:
    BIN_DIR.mkdir(exist_ok=True)
    subprocess.run(
        ["go", "build", "-o", str(BIN_PATH), "./cmd/mcp-memory-server"],
        cwd=str(REPO_ROOT),
        check=True,
    )
    return BIN_PATH


@pytest.fixture(scope="session")
def server_bin() -> Path:
    return build_server_binary()


def temp_db_env(tmp_path: Path) -> dict:
    env = os.environ.copy()
    env["MEMORY_DB_PATH"] = str(tmp_path / "memory.db")
    return env


def get_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def wait_port(host: str, port: int, timeout: float = 5.0):
    import time as _t

    deadline = _t.time() + timeout
    while _t.time() < deadline:
        try:
            with socket.create_connection((host, port), timeout=0.2):
                return
        except OSError:
            _t.sleep(0.05)
    raise TimeoutError(f"Timeout waiting for {host}:{port}")


@pytest.fixture(params=["stdio", "http"], scope="function")
def client(request, server_bin: Path, tmp_path: Path):
    transport = request.param
    env = temp_db_env(tmp_path)
    if transport == "stdio":
        proc = subprocess.Popen(
            [str(server_bin)],
            cwd=str(REPO_ROOT),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
            text=True,
            bufsize=1,
        )
        c = MCPStdioClient(proc)
        try:
            c.initialize(); c.send_initialized()
            yield c
        finally:
            c.close()
    else:
        port_path = tmp_path / "port.txt"
        proc = subprocess.Popen(
            [str(server_bin), "-http", ":0", "-portfile", str(port_path)],
            cwd=str(REPO_ROOT),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
            text=True,
        )
        try:
            for _ in range(100):
                if port_path.exists():
                    break
                import time as _t
                _t.sleep(0.05)
            port = int(port_path.read_text())
            wait_port("127.0.0.1", port)
            c = MCPHTTPStreamableClient("127.0.0.1", port)
            c.initialize(); c.send_initialized()
            yield c
        finally:
            try:
                proc.terminate()
            except Exception:
                pass
            try:
                proc.wait(timeout=5)
            except Exception:
                try:
                    proc.kill()
                except Exception:
                    pass


def _parse_text_json(client, r):
    try:
        from .http_client import MCPHTTPStreamableClient

        return MCPHTTPStreamableClient.parse_text_json(r)
    except Exception:
        from .mcp_client import MCPStdioClient

        return MCPStdioClient.parse_text_json(r)


def test_unmarshal_unknown_field_protocol_error(client):
    r = client.tools_call("create_entities", {"wrongField": []})
    assert "error" in r
    assert "unknown field" in r["error"].get("message", "")


def test_wrong_type_protocol_error(client):
    r = client.tools_call("create_entities", {"entities": [{"name": 123}]})
    assert "error" in r
    # json.Unmarshal error text varies slightly; check a stable substring
    assert "unmarshal" in r["error"].get("message", "").lower()


def test_tool_error_is_error_flag(client):
    r = client.tools_call("add_observations", {"observations": [{"entityName": "Nope", "contents": ["x"]}]})
    res = r.get("result", {})
    assert res.get("isError") is True
    # optional: verify message content presence
    content = res.get("content", [])
    assert any(isinstance(x, dict) and x.get("type") == "text" for x in content)


def test_duplicate_relations_skipped(client):
    ents = [
        {"name": "A", "entityType": "n", "observations": []},
        {"name": "B", "entityType": "n", "observations": []},
    ]
    _ = _parse_text_json(client, client.tools_call("create_entities", {"entities": ents}))
    rel = [{"from": "A", "to": "B", "relationType": "likes"}]
    first = _parse_text_json(client, client.tools_call("create_relations", {"relations": rel}))
    assert first and len(first) == 1
    second = _parse_text_json(client, client.tools_call("create_relations", {"relations": rel}))
    assert second == []


def test_delete_nonexistent_entity_ok(client):
    r = client.tools_call("delete_entities", {"entityNames": ["Ghost"]})
    assert "error" not in r


def test_delete_nonexistent_observation_ok(client):
    _ = _parse_text_json(
        client,
        client.tools_call(
            "create_entities",
            {"entities": [{"name": "C", "entityType": "n", "observations": []}]},
        ),
    )
    r = client.tools_call(
        "delete_observations",
        {"deletions": [{"entityName": "C", "observations": ["nope"]}]},
    )
    assert "error" not in r


def test_open_nodes_skips_missing(client):
    _ = _parse_text_json(
        client,
        client.tools_call(
            "create_entities",
            {"entities": [
                {"name": "O1", "entityType": "n", "observations": []},
                {"name": "O2", "entityType": "n", "observations": []},
            ]},
        ),
    )
    r = client.tools_call("open_nodes", {"names": ["O1", "Missing"]})
    data = _parse_text_json(client, r)
    assert {e["name"] for e in data["entities"]} == {"O1"}
