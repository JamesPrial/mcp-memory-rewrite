import os
import time
import socket
import subprocess
from pathlib import Path
from statistics import median
from typing import List

import pytest

from .mcp_client import MCPStdioClient
from .http_client import MCPHTTPStreamableClient
from .sse_client import MCPSSEClient


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


def _create_entities(client, entities):
    r = client.tools_call("create_entities", {"entities": entities})
    # Robust parsing: if no structured body or not JSON, treat as empty.
    if not r:
        return []
    try:
        from .http_client import MCPHTTPStreamableClient

        return MCPHTTPStreamableClient.parse_text_json(r)
    except Exception:
        try:
            from .mcp_client import MCPStdioClient

            return MCPStdioClient.parse_text_json(r)
        except Exception:
            return []


def _prep_http(server_bin: Path, tmp_path: Path):
    env = temp_db_env(tmp_path)
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
    # wait for portfile
    for _ in range(100):
        if port_path.exists():
            break
        import time as _t
        _t.sleep(0.05)
    port = int(port_path.read_text())
    wait_port("127.0.0.1", port)
    return proc, port


def _prep_http_sse(server_bin: Path, tmp_path: Path):
    env = temp_db_env(tmp_path)
    port_path = tmp_path / "port.txt"
    proc = subprocess.Popen(
        [str(server_bin), "-http", ":0", "-sse", "-portfile", str(port_path)],
        cwd=str(REPO_ROOT),
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
    )
    for _ in range(100):
        if port_path.exists():
            break
        import time as _t
        _t.sleep(0.05)
    port = int(port_path.read_text())
    wait_port("127.0.0.1", port)
    return proc, port


def _cleanup_proc(proc: subprocess.Popen):
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


def _scale(base: int) -> int:
    try:
        s = int(os.getenv("E2E_BENCH_SCALE", "1"))
    except Exception:
        s = 1
    return max(1, s) * base


@pytest.mark.benchmark
@pytest.mark.parametrize("transport", ["stdio", "http", "sse"])
def test_benchmark_create_entities_scaling(server_bin: Path, tmp_path: Path, transport: str):
    N = _scale(500)  # entities
    batch = 250

    if transport == "stdio":
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
            client.initialize(); client.send_initialized()
            # batches
            remaining = N
            t0 = time.perf_counter()
            i = 0
            while remaining > 0:
                m = min(batch, remaining)
                ents = [
                    {"name": f"E{i+j}", "entityType": "node", "observations": [f"o{i+j}"]}
                    for j in range(m)
                ]
                _create_entities(client, ents)
                remaining -= m
                i += m
            dt = time.perf_counter() - t0
            rate = N / dt if dt > 0 else float("inf")
            print(f"[stdio] create_entities N={N} dt={dt:.3f}s rate={rate:.1f}/s")
        finally:
            client.close()
    elif transport == "http":
        proc, port = _prep_http(server_bin, tmp_path)
        try:
            client = MCPHTTPStreamableClient("127.0.0.1", port)
            client.initialize(); client.send_initialized()
            remaining = N
            t0 = time.perf_counter()
            i = 0
            while remaining > 0:
                m = min(batch, remaining)
                ents = [
                    {"name": f"E{i+j}", "entityType": "node", "observations": [f"o{i+j}"]}
                    for j in range(m)
                ]
                _create_entities(client, ents)
                remaining -= m
                i += m
            dt = time.perf_counter() - t0
            rate = N / dt if dt > 0 else float("inf")
            print(f"[http] create_entities N={N} dt={dt:.3f}s rate={rate:.1f}/s")
        finally:
            _cleanup_proc(proc)
    else:  # sse
        proc, port = _prep_http_sse(server_bin, tmp_path)
        try:
            client = MCPSSEClient("127.0.0.1", port)
            client.initialize(); client.send_initialized()
            remaining = N
            t0 = time.perf_counter()
            i = 0
            while remaining > 0:
                m = min(batch, remaining)
                ents = [
                    {"name": f"E{i+j}", "entityType": "node", "observations": [f"o{i+j}"]}
                    for j in range(m)
                ]
                _create_entities(client, ents)
                remaining -= m
                i += m
            dt = time.perf_counter() - t0
            rate = N / dt if dt > 0 else float("inf")
            print(f"[sse] create_entities N={N} dt={dt:.3f}s rate={rate:.1f}/s")
        finally:
            _cleanup_proc(proc)


@pytest.mark.benchmark
def test_benchmark_search_latency_http(server_bin: Path, tmp_path: Path):
    # Build a dataset, then sample many searches to assess distribution
    N = _scale(800)
    Q = _scale(50)  # number of queries
    proc, port = _prep_http(server_bin, tmp_path)
    try:
        client = MCPHTTPStreamableClient("127.0.0.1", port)
        client.initialize(); client.send_initialized()
        # load data
        ents = [
            {"name": f"E{i}", "entityType": "kind{i%7}", "observations": [f"obs-{i}", f"tag-{i%13}"]}
            for i in range(N)
        ]
        _create_entities(client, ents)
        # timed queries
        times: List[float] = []
        for k in range(Q):
            term = f"tag-{k%13}"
            t0 = time.perf_counter()
            r = client.tools_call("search_nodes", {"query": term})
            dt = time.perf_counter() - t0
            times.append(dt)
            # basic sanity
            assert "error" not in r
        print(
            f"[http] search_nodes N={N} Q={Q} avg={sum(times)/len(times):.4f}s p50={median(times):.4f}s p95={sorted(times)[int(0.95*len(times))-1]:.4f}s"
        )
    finally:
        _cleanup_proc(proc)


@pytest.mark.benchmark
def test_benchmark_relations_and_open_nodes_http(server_bin: Path, tmp_path: Path):
    N = _scale(600)
    proc, port = _prep_http(server_bin, tmp_path)
    try:
        client = MCPHTTPStreamableClient("127.0.0.1", port)
        client.initialize(); client.send_initialized()
        ents = [
            {"name": f"E{i}", "entityType": "node", "observations": [f"o{i}"]}
            for i in range(N)
        ]
        _create_entities(client, ents)

        # Create chain of relations E0->E1->E2->...->E(N-1)
        rels = [
            {"from": f"E{i}", "to": f"E{i+1}", "relationType": "links_to"}
            for i in range(N - 1)
        ]
        t0 = time.perf_counter()
        r = client.tools_call("create_relations", {"relations": rels})
        dt_rel = time.perf_counter() - t0
        assert "error" not in r
        print(f"[http] create_relations edges={len(rels)} dt={dt_rel:.3f}s rate={(len(rels)/dt_rel):.1f}/s")

        # open_nodes over half of them
        sample_names = [f"E{i}" for i in range(0, N, 2)]
        t1 = time.perf_counter()
        r2 = client.tools_call("open_nodes", {"names": sample_names})
        dt_open = time.perf_counter() - t1
        assert "error" not in r2
        print(f"[http] open_nodes k={len(sample_names)} dt={dt_open:.3f}s")
    finally:
        _cleanup_proc(proc)


@pytest.mark.benchmark
def test_benchmark_concurrent_http_sessions(server_bin: Path, tmp_path: Path):
    # Simulate concurrency with multiple independent sessions (4 workers)
    per = _scale(250)
    workers = 4
    proc, port = _prep_http(server_bin, tmp_path)
    try:
        def worker(idx: int):
            c = MCPHTTPStreamableClient("127.0.0.1", port)
            c.initialize(); c.send_initialized()
            ents = [
                {"name": f"W{idx}_E{i}", "entityType": "node", "observations": [f"o{i}"]}
                for i in range(per)
            ]
            _create_entities(c, ents)

        import threading

        t0 = time.perf_counter()
        threads = [threading.Thread(target=worker, args=(i,)) for i in range(workers)]
        for t in threads: t.start()
        for t in threads: t.join()
        dt = time.perf_counter() - t0
        total = per * workers
        print(f"[http] concurrent sessions workers={workers} total={total} dt={dt:.3f}s rate={(total/dt):.1f}/s")
    finally:
        _cleanup_proc(proc)


@pytest.mark.benchmark
def test_benchmark_db_size_http(server_bin: Path, tmp_path: Path):
    # Measure resulting DB file size after dataset creation
    N = _scale(1200)
    db_path = tmp_path / "memory.db"
    proc, port = _prep_http(server_bin, tmp_path)
    try:
        client = MCPHTTPStreamableClient("127.0.0.1", port)
        client.initialize(); client.send_initialized()
        ents = [
            {"name": f"E{i}", "entityType": "node", "observations": [f"o{i}", f"tag-{i%17}"]}
            for i in range(N)
        ]
        _create_entities(client, ents)
        size_bytes = db_path.stat().st_size if db_path.exists() else 0
        per_entity = size_bytes / max(1, N)
        print(f"[http] db_size N={N} size={size_bytes/1024:.1f}KiB per_entity={per_entity:.1f}B")
    finally:
        _cleanup_proc(proc)
