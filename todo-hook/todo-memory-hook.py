#!/Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/venv/bin/python3
"""
Todo Memory Hook for Claude Code
Syncs TodoWrite tool calls to MCP Memory Server
Uses pre-initialized session from mcp-session-init.py
"""

import json
import sys
import os
from datetime import datetime
import hashlib
import requests
from typing import List, Dict, Any

# Configuration
SESSION_FILE = os.path.expanduser("~/.mcp-todo-session.json")
MCP_SERVER_URL = os.getenv("MCP_MEMORY_URL", "http://10.11.12.70:1818")
MCP_SESSION_ID = None  # Will be loaded from session file

class TodoMemorySync:
    def __init__(self):
        self.server_url = MCP_SERVER_URL
        self.session_id = self.load_session_id()
        self.session_name = None
        self.list_name = None

    def load_session_id(self):
        """Load session ID from file created by mcp-session-init.py"""
        try:
            if os.path.exists(SESSION_FILE):
                with open(SESSION_FILE, 'r') as f:
                    session_info = json.load(f)
                    # Use server URL from session file if available
                    if 'server_url' in session_info:
                        self.server_url = session_info['server_url']
                    return session_info.get('session_id', 'todo-hook-session')
        except Exception as e:
            print(f"Warning: Could not load session file: {e}", file=sys.stderr)

        # Fallback to environment variable or default
        return os.getenv("MCP_SESSION_ID", "todo-hook-session")

    def verify_session(self):
        """Verify the session is still valid by making a simple request"""
        try:
            # Try a simple tool call to verify session
            response = requests.post(
                f"{self.server_url}/mcp/stream",
                headers={
                    "Content-Type": "application/json",
                    "Mcp-Session-Id": self.session_id
                },
                json={
                    "jsonrpc": "2.0",
                    "method": "tools/list",
                    "id": 1,
                    "params": {}
                },
                timeout=2
            )
            return response.status_code == 200
        except:
            return False

    def generate_todo_id(self, content: str, timestamp: str) -> str:
        """Generate a unique, stable ID for a todo item"""
        # Use hash of content + timestamp for stable IDs
        hash_input = f"{content}_{timestamp}"
        return f"Todo_{hashlib.md5(hash_input.encode()).hexdigest()[:8]}"

    def call_mcp_tool(self, tool_name: str, arguments: Dict[str, Any]) -> Dict:
        """Call an MCP tool via HTTP transport"""
        try:
            response = requests.post(
                f"{self.server_url}/mcp/stream",
                headers={
                    "Content-Type": "application/json",
                    "Mcp-Session-Id": self.session_id
                },
                json={
                    "jsonrpc": "2.0",
                    "method": "tools/call",
                    "id": datetime.now().timestamp(),
                    "params": {
                        "name": tool_name,
                        "arguments": arguments
                    }
                }
            )
            return response.json()
        except Exception as e:
            return {"error": str(e)}

    def search_existing_session(self) -> str:
        """Search for today's active session"""
        today = datetime.now().strftime("%Y-%m-%d")
        result = self.call_mcp_tool(
            "search_nodes",
            {"query": f"Session_{today} AND active"}
        )

        if result and "result" in result:
            nodes = result.get("result", {}).get("nodes", [])
            if nodes:
                return nodes[0]["name"]
        return None

    def create_or_update_session(self) -> str:
        """Create a new session or reuse today's session"""
        # Check for existing session
        existing = self.search_existing_session()
        if existing:
            self.session_name = existing
            # Add observation about update
            self.call_mcp_tool(
                "add_observations",
                {
                    "observations": [{
                        "entityName": self.session_name,
                        "contents": [f"Updated at {datetime.now().isoformat()}"]
                    }]
                }
            )
        else:
            # Create new session
            today = datetime.now().strftime("%Y-%m-%d")
            self.session_name = f"Session_{today}_{datetime.now().strftime('%H%M%S')}"
            self.call_mcp_tool(
                "create_entities",
                {
                    "entities": [{
                        "name": self.session_name,
                        "entityType": "session",
                        "observations": [
                            f"Started at {datetime.now().isoformat()}",
                            f"Date: {today}",
                            "Status: active"
                        ]
                    }]
                }
            )
        return self.session_name

    def sync_todos(self, todos: List[Dict[str, str]]):
        """Sync todos to memory server"""
        timestamp = datetime.now().isoformat()

        # Create or update session
        self.create_or_update_session()

        # Create todo list for this update
        self.list_name = f"TodoList_{datetime.now().strftime('%Y%m%d_%H%M%S')}"

        # Stats for the list
        pending_count = sum(1 for t in todos if t["status"] == "pending")
        in_progress_count = sum(1 for t in todos if t["status"] == "in_progress")
        completed_count = sum(1 for t in todos if t["status"] == "completed")

        # Create list entity
        self.call_mcp_tool(
            "create_entities",
            {
                "entities": [{
                    "name": self.list_name,
                    "entityType": "todolist",
                    "observations": [
                        f"Created at {timestamp}",
                        f"Total items: {len(todos)}",
                        f"Pending: {pending_count}",
                        f"In progress: {in_progress_count}",
                        f"Completed: {completed_count}"
                    ]
                }]
            }
        )

        # Create todo entities
        todo_entities = []
        for i, todo in enumerate(todos):
            todo_id = self.generate_todo_id(todo["content"], timestamp)

            observations = [
                f"Content: {todo['content']}",
                f"Status: {todo['status']}",
                f"Active form: {todo['activeForm']}",
                f"Position: {i + 1}",
                f"Created: {timestamp}"
            ]

            # Add completion time if completed
            if todo["status"] == "completed":
                observations.append(f"Completed at: {timestamp}")

            todo_entities.append({
                "name": todo_id,
                "entityType": "todo",
                "observations": observations
            })

        if todo_entities:
            self.call_mcp_tool("create_entities", {"entities": todo_entities})

            # Create relations
            relations = []
            for entity in todo_entities:
                # Todo belongs to list
                relations.append({
                    "from": entity["name"],
                    "to": self.list_name,
                    "relationType": "belongs_to"
                })
                # Todo part of session
                relations.append({
                    "from": entity["name"],
                    "to": self.session_name,
                    "relationType": "part_of"
                })

            # List belongs to session
            relations.append({
                "from": self.list_name,
                "to": self.session_name,
                "relationType": "created_in"
            })

            self.call_mcp_tool("create_relations", {"relations": relations})

        return {
            "session": self.session_name,
            "list": self.list_name,
            "todos_synced": len(todos),
            "timestamp": timestamp
        }

def main():
    """Main entry point for the hook"""
    try:
        # Read input from stdin
        input_data = sys.stdin.read()

        # Parse the JSON - the hook receives the full tool call
        tool_call = json.loads(input_data)

        # Extract todos from the PostToolUse hook format
        # The structure is: tool_input -> todos (for PostToolUse)
        # or: arguments -> todos (for PreToolUse)
        if "tool_input" in tool_call:
            # PostToolUse format
            todos = tool_call["tool_input"].get("todos", [])
        elif "arguments" in tool_call:
            # PreToolUse format
            todos = tool_call["arguments"].get("todos", [])
        elif "todos" in tool_call:
            # Direct todo list format
            todos = tool_call["todos"]
        else:
            # Try to parse as direct todo list
            todos = json.loads(input_data) if isinstance(json.loads(input_data), list) else []

        if not todos:
            print(json.dumps({"status": "warning", "message": "No todos found in input"}))
            return

        # Initialize syncer
        syncer = TodoMemorySync()

        # Verify session is valid (already initialized by mcp-session-init.py)
        if not syncer.verify_session():
            print(json.dumps({
                "status": "error",
                "message": "Session invalid or expired. Run 'python3 mcp-session-init.py' first"
            }))
            sys.exit(1)

        # Sync todos
        result = syncer.sync_todos(todos)

        # Output result
        print(json.dumps({
            "status": "success",
            "message": f"Synced {result['todos_synced']} todos",
            "details": result
        }))

    except Exception as e:
        # Output error
        print(json.dumps({
            "status": "error",
            "message": str(e)
        }))
        sys.exit(1)

if __name__ == "__main__":
    main()