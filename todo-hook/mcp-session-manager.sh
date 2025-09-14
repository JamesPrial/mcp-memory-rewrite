#!/bin/bash
# MCP Session Manager - Handles session lifecycle
# Usage: ./mcp-session-manager.sh [start|stop|status|restart]

SESSION_FILE="$HOME/.mcp-todo-session.json"
MCP_URL="${MCP_MEMORY_URL:-http://10.11.12.70:1818}"

start_session() {
    if [ -f "$SESSION_FILE" ]; then
        echo "Session already exists. Reusing existing session."
        exit 0
    fi

    echo "Starting MCP session..."
    MCP_MEMORY_URL="$MCP_URL" /Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/venv/bin/python3 /Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/mcp-session-init.py
}

stop_session() {
    if [ ! -f "$SESSION_FILE" ]; then
        echo "No active session found"
        exit 0
    fi

    echo "Stopping MCP session..."
    /Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/venv/bin/python3 /Users/jamesprial/claude/mcp-memory-rewrite/todo-hook/mcp-session-close.py
}

session_status() {
    if [ -f "$SESSION_FILE" ]; then
        echo "Session is ACTIVE"
        echo "Session details:"
        cat "$SESSION_FILE" | ~/claude/mcp-memory-rewrite/todo-hook//venv/bin/python3 -m json.tool
    else
        echo "Session is INACTIVE"
    fi
}

restart_session() {
    echo "Restarting MCP session..."
    stop_session 2>/dev/null || true
    start_session
}

case "$1" in
    start)
        start_session
        ;;
    stop)
        stop_session
        ;;
    status)
        session_status
        ;;
    restart)
        restart_session
        ;;
    *)
        echo "Usage: $0 {start|stop|status|restart}"
        echo ""
        echo "Environment variables:"
        echo "  MCP_MEMORY_URL - MCP server URL (default: http://10.11.12.70:1818)"
        exit 1
        ;;
esac