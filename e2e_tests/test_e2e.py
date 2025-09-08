import os
import sys
import json
import time
import tempfile
import socket
import subprocess
from pathlib import Path

import pytest

from .mcp_client import MCPStdioClient
from .http_client import MCPHTTPStreamableClient
from .sse_client import MCPSSEClient


REPO_ROOT = Path(__file__).resolve().parents[1]
BIN_DIR = REPO_ROOT / ".e2e_bin"
BIN_PATH = BIN_DIR / "mcp-memory-server"


def build_server_binary() -> Path:
    BIN_DIR.mkdir(exist_ok=True)
    # Build the Go server binary
    subprocess.run(
        [
            "go",
            "build",
            "-o",
            str(BIN_PATH),
            "./cmd/mcp-memory-server",
        ],
        cwd=str(REPO_ROOT),
        check=True,
    )
    return BIN_PATH


@pytest.fixture(scope="session")
def server_bin() -> Path:
    return build_server_binary()


def temp_db_env(tmp_path: Path) -> dict:
    db_path = tmp_path / "memory.db"
    env = os.environ.copy()
    env["MEMORY_DB_PATH"] = str(db_path)
    return env


def wait_port(host: str, port: int, timeout: float = 5.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection((host, port), timeout=0.2):
                return
        except OSError:
            time.sleep(0.05)
    raise TimeoutError(f"Timeout waiting for {host}:{port}")


def get_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


@pytest.fixture()
def stdio_server(server_bin: Path, tmp_path: Path):
    env = temp_db_env(tmp_path)
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
    try:
        client = MCPStdioClient(proc)
        yield client
    finally:
        client.close()
        try:
            if proc.stderr:
                _ = proc.stderr.read()
        except Exception:
            pass


@pytest.fixture()
def http_server(server_bin: Path, tmp_path: Path):
    env = temp_db_env(tmp_path)
    port = get_free_port()
    addr = f"127.0.0.1:{port}"
    proc = subprocess.Popen(
        [str(server_bin), "-http", addr],
        cwd=str(REPO_ROOT),
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
    )
    try:
        wait_port("127.0.0.1", port)
        client = MCPHTTPStreamableClient("127.0.0.1", port)
        yield client
    finally:
        # Try graceful shutdown via SIGTERM
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
        try:
            if proc.stderr:
                _ = proc.stderr.read()
        except Exception:
            pass


def _happy_path_flow(client, is_http: bool):
    # initialize + initialized
    init_resp = client.initialize()
    client.send_initialized()

    # tools/list should include our server tools
    tl = client.tools_list()
    assert "result" in tl and isinstance(tl["result"].get("tools"), list)
    tool_names = {t.get("name") for t in tl["result"]["tools"]}
    exp = {
        "create_entities",
        "create_relations",
        "add_observations",
        "delete_entities",
        "delete_observations",
        "delete_relations",
        "read_graph",
        "search_nodes",
        "open_nodes",
    }
    assert exp.issubset(tool_names)

    # create_entities
    ents = [
        {
            "name": "Alice",
            "entityType": "person",
            "observations": ["Loves tea", "Engineer"],
        },
        {
            "name": "Bob",
            "entityType": "person",
            "observations": ["Prefers coffee"],
        },
        {
            "name": "AcmeCorp",
            "entityType": "organization",
            "observations": ["Founded 1990"],
        },
    ]
    r = client.tools_call("create_entities", {"entities": ents})
    created = (client.parse_text_json(r) if is_http else MCPStdioClient.parse_text_json(r))
    assert isinstance(created, list) and len(created) == 3

    # create_relations (Alice works_at AcmeCorp), and one unknown entity (skipped)
    rels = [
        {"from": "Alice", "to": "AcmeCorp", "relationType": "works_at"},
        {"from": "Bob", "to": "AcmeCorp", "relationType": "applies_to"},
        {"from": "Ghost", "to": "AcmeCorp", "relationType": "works_at"},
    ]
    rr = client.tools_call("create_relations", {"relations": rels})
    created_rels = (client.parse_text_json(rr) if is_http else MCPStdioClient.parse_text_json(rr))
    # Only 2 should be created; relation involving unknown entity is skipped
    assert isinstance(created_rels, list) and len(created_rels) == 2

    # read_graph
    rg = client.tools_call("read_graph")
    graph = (client.parse_text_json(rg) if is_http else MCPStdioClient.parse_text_json(rg))
    assert {e["name"] for e in graph["entities"]} >= {"Alice", "Bob", "AcmeCorp"}
    assert any(r["relationType"] == "works_at" for r in graph["relations"])

    # search_nodes
    sn = client.tools_call("search_nodes", {"query": "coffee"})
    search_graph = (client.parse_text_json(sn) if is_http else MCPStdioClient.parse_text_json(sn))
    assert any(e["name"] == "Bob" for e in search_graph["entities"])

    # open_nodes
    on = client.tools_call("open_nodes", {"names": ["Alice", "AcmeCorp"]})
    open_graph = (client.parse_text_json(on) if is_http else MCPStdioClient.parse_text_json(on))
    assert {e["name"] for e in open_graph["entities"]} == {"Alice", "AcmeCorp"}
    assert any(r["relationType"] == "works_at" for r in open_graph["relations"])

    # add_observations
    ao = client.tools_call(
        "add_observations",
        {
            "observations": [
                {"entityName": "Alice", "contents": ["Speaks Spanish"]},
                {"entityName": "Bob", "contents": ["Runner"]},
            ]
        },
    )
    add_res = (client.parse_text_json(ao) if is_http else MCPStdioClient.parse_text_json(ao))
    assert any(x["entityName"] == "Alice" for x in add_res)

    # Duplicate add_observations should add none
    ao2 = client.tools_call(
        "add_observations",
        {"observations": [{"entityName": "Alice", "contents": ["Speaks Spanish"]}]},
    )
    add_res2 = (client.parse_text_json(ao2) if is_http else MCPStdioClient.parse_text_json(ao2))
    assert add_res2 and add_res2[0].get("addedObservations") == []

    # delete_observations
    do = client.tools_call(
        "delete_observations",
        {"deletions": [{"entityName": "Alice", "observations": ["Loves tea"]}]},
    )
    # Text response without body; just ensure no error
    assert "error" not in do

    # delete_relations for applies_to; ensure removed
    dr = client.tools_call(
        "delete_relations",
        {"relations": [{"from": "Bob", "to": "AcmeCorp", "relationType": "applies_to"}]},
    )
    assert "error" not in dr
    rg_rel = client.tools_call("read_graph")
    graph_rel = (client.parse_text_json(rg_rel) if is_http else MCPStdioClient.parse_text_json(rg_rel))
    assert not any(r["relationType"] == "applies_to" for r in graph_rel["relations"])

    # delete_entities
    de = client.tools_call("delete_entities", {"entityNames": ["Bob"]})
    assert "error" not in de

    # final graph checks
    rg2 = client.tools_call("read_graph")
    graph2 = (client.parse_text_json(rg2) if is_http else MCPStdioClient.parse_text_json(rg2))
    names = {e["name"] for e in graph2["entities"]}
    assert "Bob" not in names and "Alice" in names and "AcmeCorp" in names

    # Re-create same entities should only re-create deleted entity (Bob)
    r_dup = client.tools_call("create_entities", {"entities": ents})
    created_dup = (client.parse_text_json(r_dup) if is_http else MCPStdioClient.parse_text_json(r_dup))
    names_dup = {e.get("name") for e in (created_dup or [])}
    assert names_dup == {"Bob"}


def _error_and_misuse(client, is_http: bool):
    client.initialize()
    client.send_initialized()

    # error: add_observations for missing entity should return tool error (isError=true)
    bad = client.tools_call(
        "add_observations",
        {"observations": [{"entityName": "DoesNotExist", "contents": ["x"]}]},
    )
    res = bad.get("result", {})
    assert res.get("isError") is True

    # misuse: unknown field should be rejected by schema/unmarshal (protocol-level error)
    bad2 = client.tools_call("create_entities", {"wrongField": []})
    assert "error" in bad2

    # misuse: wrong types -> schema/unmarshal error at protocol level
    bad3 = client.tools_call("create_entities", {"entities": [{"name": 123}]})
    assert "error" in bad3


def _benchmark_create_entities(client, is_http: bool, n: int = 300):
    client.initialize()
    client.send_initialized()
    ents = [
        {"name": f"E{i}", "entityType": "node", "observations": [f"o{i}"]}
        for i in range(n)
    ]
    t0 = time.time()
    r = client.tools_call("create_entities", {"entities": ents})
    dt = time.time() - t0
    if "error" in r:
        raise AssertionError(f"benchmark call failed: {r}")
    rate = n / dt if dt > 0 else float("inf")
    print(f"benchmark_create_entities: n={n} dt={dt:.3f}s rate={rate:.1f}/s")


def test_stdio_happy_path(stdio_server):
    _happy_path_flow(stdio_server, is_http=False)


def test_stdio_error_and_misuse(stdio_server):
    _error_and_misuse(stdio_server, is_http=False)


def test_http_happy_path(http_server):
    _happy_path_flow(http_server, is_http=True)


def test_http_error_and_misuse(http_server):
    _error_and_misuse(http_server, is_http=True)


@pytest.fixture()
def http_sse_server(server_bin: Path, tmp_path: Path):
    env = temp_db_env(tmp_path)
    port = get_free_port()
    addr = f"127.0.0.1:{port}"
    proc = subprocess.Popen(
        [str(server_bin), "-http", addr, "-sse"],
        cwd=str(REPO_ROOT),
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
    )
    try:
        wait_port("127.0.0.1", port)
        client = MCPSSEClient("127.0.0.1", port)
        yield client
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
        try:
            if proc.stderr:
                _ = proc.stderr.read()
        except Exception:
            pass


def test_sse_happy_path(http_sse_server):
    _happy_path_flow(http_sse_server, is_http=True)


def test_sse_error_and_misuse(http_sse_server):
    _error_and_misuse(http_sse_server, is_http=True)


@pytest.mark.parametrize("mode", ["stdio", "http"])  # simple benchmark in both modes
def test_benchmark_create_entities(server_bin: Path, tmp_path: Path, mode: str):
    if mode == "stdio":
        env = temp_db_env(tmp_path)
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
        client = MCPStdioClient(proc)
        try:
            _benchmark_create_entities(client, is_http=False, n=300)
        finally:
            client.close()
    else:
        env = temp_db_env(tmp_path)
        port = get_free_port()
        proc = subprocess.Popen(
            [str(server_bin), "-http", f"127.0.0.1:{port}"],
            cwd=str(REPO_ROOT),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
            text=True,
        )
        try:
            wait_port("127.0.0.1", port)
            client = MCPHTTPStreamableClient("127.0.0.1", port)
            _benchmark_create_entities(client, is_http=True, n=300)
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
