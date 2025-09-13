#!/usr/bin/env python3
"""Test the Go MCP memory server with 1 million entities."""

import json
import subprocess
import time
import os
import sys
from pathlib import Path
import tempfile
import random
import socket
import urllib.request
import urllib.error
import shutil

class MCPBenchmarkClient:
    """A self-contained, minimal MCP client for benchmarking."""

    def __init__(self, host, port):
        self.url = f"http://{host}:{port}/mcp/stream"
        self._request_id = 0

    def _send_request(self, method, params, is_notification=False):
        self._request_id += 1
        payload = {"jsonrpc": "2.0", "method": method}
        if params:
            payload["params"] = params
        if not is_notification:
            payload["id"] = self._request_id

        data = json.dumps(payload).encode("utf-8")
        headers = {"Content-Type": "application/json"}
        req = urllib.request.Request(
            self.url, data=data, headers=headers, method="POST"
        )

        try:
            with urllib.request.urlopen(req) as response:
                if response.status != 200:
                    raise RuntimeError(
                        f"Request failed: {response.status} {response.read().decode()}"
                    )
                if is_notification:
                    return None
                response_data = json.loads(response.read().decode("utf-8"))
                if "error" in response_data:
                    raise RuntimeError(f"RPC Error: {response_data['error']}")
                return response_data.get("result")
        except urllib.error.URLError as e:
            raise RuntimeError(f"Failed to connect to server at {self.url}: {e}")

    def initialize(self):
        params = {"processId": os.getpid(), "clientInfo": {"name": "benchmark-client"}}
        return self._send_request("initialize", params)

    def send_initialized(self):
        return self._send_request("initialized", {}, is_notification=True)

    def tools_call(self, tool_name, params):
        method = f"tools/{tool_name}"
        return self._send_request(method, params)

REPO_ROOT = Path(__file__).resolve().parent
BIN_DIR = REPO_ROOT / ".e2e_bin"
BIN_PATH = BIN_DIR / "mcp-memory-server"

def build_server():
    """Build the Go server binary."""
    print("Building server...")
    BIN_DIR.mkdir(exist_ok=True)
    subprocess.run(
        ["go", "build", "-o", str(BIN_PATH), "./cmd/mcp-memory-server"],
        cwd=str(REPO_ROOT),
        check=True,
    )
    return BIN_PATH

def wait_port(host: str, port: int, timeout: float = 5.0):
    """Wait for port to be available."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection((host, port), timeout=0.2):
                return
        except OSError:
            time.sleep(0.05)
    raise TimeoutError(f"Timeout waiting for {host}:{port}")

def main():
    print("="*60)
    print("Testing Go MCP Memory Server with 1 MILLION entities")
    print("="*60)
    
    server_bin = build_server()
    tmpdir = tempfile.mkdtemp(prefix="bench_1m_")
    db_path = Path(tmpdir) / "memory.db"
    
    # Start server
    env = os.environ.copy()
    env["MEMORY_DB_PATH"] = str(db_path)
    env["LOG_LEVEL"] = "error"
    
    port_file = Path(tmpdir) / "port.txt"
    proc = subprocess.Popen(
        [str(server_bin), "-http", ":0", "-portfile", str(port_file)],
        cwd=str(REPO_ROOT),
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
    )

    try:
        # Wait for port
        for _ in range(100):
            if port_file.exists():
                break
            time.sleep(0.05)
        port = int(port_file.read_text())
        wait_port("127.0.0.1", port)

        client = MCPBenchmarkClient("127.0.0.1", port)
        client.initialize()
        client.send_initialized()

        print("\nPhase 1: Insert 1,000,000 entities in batches of 10,000")
        print("-" * 60)

        batch_size = 10000
        total_time = 0
        entities_created = 0

        for batch_start in range(0, 1000000, batch_size):
            batch_end = min(batch_start + batch_size, 1000000)
            batch_entities = []

            for i in range(batch_start, batch_end):
                batch_entities.append(
                    {
                        "name": f"entity_{i}",
                        "entityType": f"type_{i % 100}",
                        "observations": [f"obs_{i}_1", f"obs_{i}_2"],
                    }
                )

            start = time.time()
            client.tools_call("create_entities", {"entities": batch_entities})
            elapsed = time.time() - start
            total_time += elapsed

            # Simple count - assume success
            entities_created += len(batch_entities)

            if (batch_end % 100000) == 0:
                rate = entities_created / total_time if total_time > 0 else 0
                print(
                    f"  {batch_end:,} entities | {total_time:.1f}s | {rate:.0f} entities/sec"
                )

        print(f"\nCompleted: {entities_created:,} entities in {total_time:.1f}s")
        print(f"Average rate: {entities_created/total_time:.0f} entities/sec")

        # Force a sync to ensure DB is written to disk
        time.sleep(0.5)

        # Check DB size
        db_size_mb = db_path.stat().st_size / 1024 / 1024 if db_path.exists() else 0
        print(f"Database size: {db_size_mb:.1f} MB")

        # Also check for any DB files in the directory
        db_files = list(Path(tmpdir).glob("*.db*"))
        for dbf in db_files:
            size_mb = dbf.stat().st_size / 1024 / 1024
            print(f"  {dbf.name}: {size_mb:.1f} MB")

        print("\nPhase 2: Test search performance (100 queries)")
        print("-" * 60)

        search_times = []
        for i in range(100):
            search_term = f"obs_{random.randint(0, 999999)}_1"
            start = time.time()
            client.tools_call("search_nodes", {"query": search_term})
            elapsed = time.time() - start
            search_times.append(elapsed)

            if i % 20 == 0:
                avg_so_far = sum(search_times) / len(search_times)
                print(f"  Query {i+1}/100: avg {avg_so_far*1000:.1f}ms")

        avg_time = sum(search_times) / len(search_times)
        p50_time = sorted(search_times)[50]
        p95_time = sorted(search_times)[95]
        p99_time = sorted(search_times)[99]

        print(f"\nSearch Results:")
        print(f"  Average: {avg_time*1000:.1f}ms")
        print(f"  P50: {p50_time*1000:.1f}ms")
        print(f"  P95: {p95_time*1000:.1f}ms")
        print(f"  P99: {p99_time*1000:.1f}ms")

        # Save results
        results = {
            "entities": 1000000,
            "insert_time_s": total_time,
            "insert_rate": entities_created / total_time,
            "db_size_mb": db_size_mb,
            "search_avg_ms": avg_time * 1000,
            "search_p50_ms": p50_time * 1000,
            "search_p95_ms": p95_time * 1000,
            "search_p99_ms": p99_time * 1000,
        }

        with open("benchmark_1m_results.json", "w") as f:
            json.dump(results, f, indent=2)

        print("\nResults saved to benchmark_1m_results.json")

    finally:
        # Cleanup
        print("\nCleaning up server and temporary files...")
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            print("Server did not terminate gracefully, killing.")
            proc.kill()
            proc.wait()
        
        shutil.rmtree(tmpdir)
        print("Cleanup complete.")

if __name__ == "__main__":
    main()