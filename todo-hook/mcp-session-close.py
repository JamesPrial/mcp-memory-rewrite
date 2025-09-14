#!/Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/venv/bin/python3
"""
MCP Session Cleanup - Close active session
Usage: python3 mcp-session-close.py
"""

import requests
import json
import os
import sys
from datetime import datetime

SESSION_FILE = os.path.expanduser("~/.mcp-todo-session.json")

def close_session():
    """Close the MCP session and clean up"""
    try:
        # Load session info
        if not os.path.exists(SESSION_FILE):
            print("No active session found")
            return True

        with open(SESSION_FILE, 'r') as f:
            session_info = json.load(f)

        session_id = session_info.get("session_id")
        server_url = session_info.get("server_url")

        if not session_id or not server_url:
            print("Invalid session file")
            return False

        print(f"Closing session {session_id}...")

        # Send close notification to server
        # Note: MCP doesn't have an explicit close, but we can clean up our tracking
        try:
            # Try to mark our session entities as inactive
            response = requests.post(
                f"{server_url}/mcp/stream",
                headers={"Mcp-Session-Id": session_id},
                json={
                    "jsonrpc": "2.0",
                    "method": "tools/call",
                    "id": 1,
                    "params": {
                        "name": "search_nodes",
                        "arguments": {
                            "query": f"Session_{datetime.now().strftime('%Y-%m-%d')} AND active"
                        }
                    }
                },
                timeout=2
            )

            if response.status_code == 200:
                result = response.json()
                nodes = result.get("result", {}).get("entities", [])

                # Mark sessions as inactive
                for node in nodes:
                    requests.post(
                        f"{server_url}/mcp/stream",
                        headers={"Mcp-Session-Id": session_id},
                        json={
                            "jsonrpc": "2.0",
                            "method": "tools/call",
                            "params": {
                                "name": "add_observations",
                                "arguments": {
                                    "observations": [{
                                        "entityName": node["name"],
                                        "contents": [
                                            f"Closed at {datetime.now().isoformat()}",
                                            "Status: inactive"
                                        ]
                                    }]
                                }
                            }
                        },
                        timeout=2
                    )
                    print(f"Marked {node['name']} as inactive")

        except Exception as e:
            # Server might be down or session expired - that's ok
            print(f"Could not update session status: {e}")

        # Remove local session file
        os.remove(SESSION_FILE)
        print(f"Session file removed: {SESSION_FILE}")

        return True

    except Exception as e:
        print(f"Error closing session: {e}", file=sys.stderr)
        return False

def main():
    """Main entry point"""
    success = close_session()

    if success:
        print("Session closed successfully")
    else:
        print("Failed to close session properly", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()