# MCP Memory Server on Unraid

## Quick Start

1. In Unraid, go to **Docker** tab
2. Click **Add Container**
3. Use these settings:

### Container Settings

| Field | Value |
|-------|-------|
| **Name** | mcp-memory-server |
| **Repository** | ghcr.io/jamesprial/mcp-memory-rewrite:latest |
| **Network Type** | bridge |
| **Extra Parameters** | `-http :8080 -sse` |

### Port Mappings

| Container Port | Host Port | Description |
|----------------|-----------|-------------|
| 8080 | 8080 | HTTP API endpoint |

### Path Mappings

| Container Path | Host Path | Description |
|----------------|-----------|-------------|
| /data | /mnt/user/appdata/mcp-memory | Database storage |

### Environment Variables (Optional)

| Variable | Default | Options | Description |
|----------|---------|---------|-------------|
| LOG_LEVEL | info | debug, info, warn, error | Logging verbosity |

## Understanding the Modes

The MCP Memory Server can run in three modes:

### 1. **stdio Mode** (Default)
- Used for direct communication with MCP clients
- Not useful for Unraid web access
- Don't use this mode in Unraid

### 2. **HTTP Mode** 
- Add to Extra Parameters: `-http :8080`
- Provides REST API on port 8080
- Good for programmatic access

### 3. **HTTP with SSE Mode** (Recommended)
- Add to Extra Parameters: `-http :8080 -sse`
- Provides REST API with Server-Sent Events
- Best for real-time updates and web clients

## Accessing the Server

Once running, you can access the server at:
```
http://YOUR-UNRAID-IP:8080
```

## API Examples

### Create an Entity
```bash
curl -X POST http://YOUR-UNRAID-IP:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "create_entities",
      "arguments": {
        "entities": [{
          "name": "test_entity",
          "entityType": "test",
          "observations": ["test observation"]
        }]
      }
    },
    "id": 1
  }'
```

### Search Entities
```bash
curl -X POST http://YOUR-UNRAID-IP:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "search_nodes",
      "arguments": {
        "query": "test"
      }
    },
    "id": 1
  }'
```

## Viewing Logs

To view container logs in Unraid:
1. Go to Docker tab
2. Click on the container icon
3. Select "Logs"

Or via command line:
```bash
docker logs mcp-memory-server
```

## Backup

The SQLite database is stored at:
```
/mnt/user/appdata/mcp-memory/memory.db
```

To backup, simply copy this file while the container is stopped, or use SQLite's backup command:
```bash
sqlite3 /mnt/user/appdata/mcp-memory/memory.db ".backup /mnt/user/backups/memory_backup.db"
```

## Troubleshooting

### Container won't start
- Check logs for errors
- Ensure port 8080 is not in use by another container
- Verify the appdata directory exists and has correct permissions

### Can't connect to API
- Ensure you added `-http :8080` to Extra Parameters
- Check firewall rules
- Verify the container is running: `docker ps | grep mcp-memory`

### Database errors
- Check disk space: `df -h /mnt/user/appdata`
- Verify permissions: `ls -la /mnt/user/appdata/mcp-memory/`
- Try removing and recreating the database file (this will delete all data)

## Performance Notes

- The server can handle millions of entities efficiently
- Search performance remains sub-millisecond even at scale
- SQLite WAL mode is used for better concurrent access
- Database size grows slowly (~50MB per million entities)