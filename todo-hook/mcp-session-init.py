#!/Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/venv/bin/python3
"""
MCP Session Initializer - Run in background to establish session
Usage: python3 mcp-session-init.py &
"""

import requests
import json
import os
import sys
from datetime import datetime

SESSION_FILE = os.path.expanduser("~/.mcp-todo-session.json")
MCP_URL = os.getenv("MCP_MEMORY_URL", "http://10.11.12.70:1818")

def init_session():
    try:
        # First request - get session ID from server
        print(f"Connecting to MCP server at {MCP_URL}...")
        response = requests.post(
            f"{MCP_URL}/mcp/stream",
            json={
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "protocolVersion": "0.1.0",
                    "capabilities": {},
                    "clientInfo": {
                        "name": "todo-hook-client",
                        "version": "1.0.0"
                    }
                }
            },
            timeout=5
        )

        session_id = response.headers.get("Mcp-Session-Id")
        if not session_id:
            print("Warning: No session ID returned, server might be in stateless mode")
            session_id = "stateless"
        else:
            print(f"Got session ID: {session_id}")

            # Send initialized notification to complete handshake
            response = requests.post(
                f"{MCP_URL}/mcp/stream",
                headers={"Mcp-Session-Id": session_id},
                json={
                    "jsonrpc": "2.0",
                    "method": "notifications/initialized",
                    "params": {}
                },
                timeout=5
            )
            print("Session initialized successfully")

        # Save session info for todo hook to use
        session_info = {
            "session_id": session_id,
            "server_url": MCP_URL,
            "created": datetime.now().isoformat(),
            "pid": os.getpid()
        }

        with open(SESSION_FILE, "w") as f:
            json.dump(session_info, f, indent=2)

        print(f"Session saved to {SESSION_FILE}")
        return True

    except requests.exceptions.RequestException as e:
        print(f"Failed to initialize session: {e}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        return False

if __name__ == "__main__":
    # Run in background - just initialize and exit
    success = init_session()
    sys.exit(0 if success else 1)