#!/usr/bin/env python3
"""
MCP HTTP Bridge - Forwards stdio MCP requests to HTTP server
Usage: python3 mcp-http-bridge.py <server-url>
"""

import sys
import json
import requests
import logging

# Configure logging to stderr so it doesn't interfere with stdio
logging.basicConfig(
    level=logging.ERROR,
    format='%(asctime)s - %(levelname)s - %(message)s',
    stream=sys.stderr
)

def main():
    if len(sys.argv) != 2:
        print("Usage: python3 mcp-http-bridge.py <server-url>", file=sys.stderr)
        sys.exit(1)
    
    server_url = sys.argv[1]
    
    # Read JSON-RPC from stdin, forward to HTTP, write response to stdout
    while True:
        try:
            # Read line from stdin
            line = sys.stdin.readline()
            if not line:
                break
            
            # Parse JSON-RPC request
            request = json.loads(line.strip())
            
            # Forward to HTTP server
            response = requests.post(
                server_url,
                json=request,
                headers={'Content-Type': 'application/json'}
            )
            
            # Write response to stdout
            if response.status_code == 200:
                sys.stdout.write(response.text + '\n')
                sys.stdout.flush()
            else:
                error_response = {
                    "jsonrpc": "2.0",
                    "error": {
                        "code": -32603,
                        "message": f"HTTP error: {response.status_code}"
                    },
                    "id": request.get("id")
                }
                sys.stdout.write(json.dumps(error_response) + '\n')
                sys.stdout.flush()
                
        except json.JSONDecodeError as e:
            logging.error(f"Invalid JSON: {e}")
        except requests.RequestException as e:
            logging.error(f"HTTP request failed: {e}")
            error_response = {
                "jsonrpc": "2.0",
                "error": {
                    "code": -32603,
                    "message": f"Connection error: {str(e)}"
                },
                "id": None
            }
            sys.stdout.write(json.dumps(error_response) + '\n')
            sys.stdout.flush()
        except KeyboardInterrupt:
            break
        except Exception as e:
            logging.error(f"Unexpected error: {e}")

if __name__ == "__main__":
    main()