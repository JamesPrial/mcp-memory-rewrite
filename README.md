# Knowledge Graph Memory Server (Go Implementation with SQLite)

[![CI](https://github.com/jamesprial/mcp-memory-rewrite/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/jamesprial/mcp-memory-rewrite/actions/workflows/ci.yml)

A Go implementation of the MCP memory server using SQLite for persistent storage, providing enhanced performance and reliability over the original JSON-based implementation.

## Key Improvements

This Go implementation offers several advantages over the original TypeScript version:

- **SQLite Backend**: ACID-compliant database with better performance for large datasets
- **Full-Text Search**: Leverages SQLite FTS5 for advanced search capabilities
- **Concurrent Access**: SQLite handles multiple readers with proper locking
- **Data Integrity**: Foreign key constraints and transactions ensure consistency
- **Efficient Queries**: Indexed columns and optimized SQL queries
- **Smaller Memory Footprint**: Go's efficient memory management

## Core Concepts

### Entities
Entities are the primary nodes in the knowledge graph. Each entity has:
- A unique name (identifier)
- An entity type (e.g., "person", "organization", "event")
- A list of observations

Example:
```json
{
  "name": "John_Smith",
  "entityType": "person",
  "observations": ["Speaks fluent Spanish"]
}
```

### Relations
Relations define directed connections between entities. They are always stored in active voice and describe how entities interact or relate to each other.

Example:
```json
{
  "from": "John_Smith",
  "to": "Anthropic",
  "relationType": "works_at"
}
```

### Observations
Observations are discrete pieces of information about an entity. They are:
- Stored as strings
- Attached to specific entities
- Can be added or removed independently
- Should be atomic (one fact per observation)

Example:
```json
{
  "entityName": "John_Smith",
  "observations": [
    "Speaks fluent Spanish",
    "Graduated in 2019",
    "Prefers morning meetings"
  ]
}
```

## Installation

### Prerequisites
- Go 1.23 or later
- SQLite3 (included with most systems)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/jamesprial/mcp-memory-rewrite.git
cd mcp-memory-rewrite

# Build the server
go build -o mcp-memory-server ./cmd/mcp-memory-server

# Or install directly
go install ./cmd/mcp-memory-server
```

### Running the Server

```bash
# Run in stdio mode (default)
./mcp-memory-server

# Run in HTTP mode on port 8080
./mcp-memory-server -http :8080

# Run in HTTP mode with Server-Sent Events
./mcp-memory-server -http :8080 -sse

# Run with custom database path
MEMORY_DB_PATH=/path/to/db.sqlite ./mcp-memory-server

# Run with debug logging
LOG_LEVEL=debug ./mcp-memory-server
```

## Configuration

### Command Line Flags

- `-http <address>`: Run in HTTP mode on specified address (e.g., `:8080`)
- `-sse`: Use Server-Sent Events for HTTP mode (requires `-http`)
- `-portfile <path>`: Write the actual bound TCP port to a file (useful for testing)

### Environment Variables

- `MEMORY_DB_PATH`: Path to the SQLite database file (default: `~/.mcp-memory/memory.db`)
- `LOG_LEVEL`: Logging level - `debug`, `info`, `warn`, `error` (default: `info`)
- `LOG_FORMAT`: Log output format - `json` or `text` (default: `text`, uses `json` when `ENV=production`)
- `DEBUG`: Set to `true` for debug logging (alternative to `LOG_LEVEL=debug`)
- `ENV`: Environment mode - Set to `production` for JSON logging (default: development)

## Python Test Dependencies

Install Python deps for running E2E and benchmarks:

```bash
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## API

### Tools

- **create_entities**
  - Create multiple new entities in the knowledge graph
  - Input: `entities` (array of objects)
    - Each object contains:
      - `name` (string): Entity identifier
      - `entityType` (string): Type classification
      - `observations` (string[]): Associated observations
  - Ignores entities with existing names

- **create_relations**
  - Create multiple new relations between entities
  - Input: `relations` (array of objects)
    - Each object contains:
      - `from` (string): Source entity name
      - `to` (string): Target entity name
      - `relationType` (string): Relationship type in active voice
  - Skips duplicate relations

- **add_observations**
  - Add new observations to existing entities
  - Input: `observations` (array of objects)
    - Each object contains:
      - `entityName` (string): Target entity
      - `contents` (string[]): New observations to add
  - Returns added observations per entity
  - Fails if entity doesn't exist

- **delete_entities**
  - Remove entities and their relations
  - Input: `entityNames` (string[])
  - Cascading deletion of associated relations
  - Silent operation if entity doesn't exist

- **delete_observations**
  - Remove specific observations from entities
  - Input: `deletions` (array of objects)
    - Each object contains:
      - `entityName` (string): Target entity
      - `observations` (string[]): Observations to remove
  - Silent operation if observation doesn't exist

- **delete_relations**
  - Remove specific relations from the graph
  - Input: `relations` (array of objects)
    - Each object contains:
      - `from` (string): Source entity name
      - `to` (string): Target entity name
      - `relationType` (string): Relationship type
  - Silent operation if relation doesn't exist

- **read_graph**
  - Read the entire knowledge graph
  - No input required
  - Returns complete graph structure with all entities and relations

- **search_nodes**
  - Search for nodes based on query
  - Input: `query` (string)
  - Searches across:
    - Entity names
    - Entity types
    - Observation content
  - Uses SQLite FTS5 for efficient full-text search
  - Returns matching entities and their relations

- **open_nodes**
  - Retrieve specific nodes by name
  - Input: `names` (string[])
  - Returns:
    - Requested entities
    - Relations between requested entities
  - Silently skips non-existent nodes

## Usage with Claude Desktop

Add this to your `claude_desktop_config.json`:

### Direct Binary

```json
{
  "mcpServers": {
    "memory": {
      "command": "/path/to/mcp-memory-server",
      "args": [],
      "env": {
        "MEMORY_DB_PATH": "/path/to/custom/memory.db"
      }
    }
  }
}
```

### Using Go Run

```json
{
  "mcpServers": {
    "memory": {
      "command": "go",
      "args": ["run", "github.com/jamesprial/mcp-memory-rewrite/cmd/mcp-memory-server@latest"],
      "env": {
        "MEMORY_DB_PATH": "/path/to/custom/memory.db"
      }
    }
  }
}
```

## VS Code Installation

Add the configuration to your MCP configuration file:

**User Configuration (Recommended)**
Open the Command Palette (`Ctrl + Shift + P`) and run `MCP: Open User Configuration`.

**Workspace Configuration**
Alternatively, add to `.vscode/mcp.json` in your workspace.

### Configuration

```json
{
  "servers": {
    "memory": {
      "command": "/path/to/mcp-memory-server",
      "args": [],
      "env": {
        "MEMORY_DB_PATH": "/path/to/custom/memory.db"
      }
    }
  }
}
```

## System Prompt

Here is an example prompt for chat personalization. You could use this prompt in the "Custom Instructions" field of a Claude.ai Project:

```
Follow these steps for each interaction:

1. User Identification:
   - You should assume that you are interacting with default_user
   - If you have not identified default_user, proactively try to do so.

2. Memory Retrieval:
   - Always begin your chat by saying only "Remembering..." and retrieve all relevant information from your knowledge graph
   - Always refer to your knowledge graph as your "memory"

3. Memory
   - While conversing with the user, be attentive to any new information that falls into these categories:
     a) Basic Identity (age, gender, location, job title, education level, etc.)
     b) Behaviors (interests, habits, etc.)
     c) Preferences (communication style, preferred language, etc.)
     d) Goals (goals, targets, aspirations, etc.)
     e) Relationships (personal and professional relationships up to 3 degrees of separation)

4. Memory Update:
   - If any new information was gathered during the interaction, update your memory as follows:
     a) Create entities for recurring organizations, people, and significant events
     b) Connect them to the current entities using relations
     c) Store facts about them as observations
```

## Database Schema

The SQLite database uses the following schema:

### Tables

**entities**
- `id` (INTEGER PRIMARY KEY)
- `name` (TEXT UNIQUE)
- `entity_type` (TEXT)
- `created_at` (TIMESTAMP)
- `updated_at` (TIMESTAMP)

**observations**
- `id` (INTEGER PRIMARY KEY)
- `entity_id` (INTEGER FOREIGN KEY)
- `content` (TEXT)
- `created_at` (TIMESTAMP)

**relations**
- `id` (INTEGER PRIMARY KEY)
- `from_entity_id` (INTEGER FOREIGN KEY)
- `to_entity_id` (INTEGER FOREIGN KEY)
- `relation_type` (TEXT)
- `created_at` (TIMESTAMP)

### Full-Text Search Tables

- `entities_fts` - FTS5 virtual table for entity search
- `observations_fts` - FTS5 virtual table for observation search

## Docker Usage

### Using Pre-built Image

The Docker image is automatically built and published to GitHub Container Registry:

```bash
# Pull the latest image
docker pull ghcr.io/jamesprial/mcp-memory-rewrite:latest

# Run in stdio mode (for MCP clients)
docker run -i --rm \
  -v mcp-memory-data:/data \
  ghcr.io/jamesprial/mcp-memory-rewrite:latest

# Run in HTTP mode on port 8080
docker run -d \
  -p 8080:8080 \
  -v mcp-memory-data:/data \
  ghcr.io/jamesprial/mcp-memory-rewrite:latest -http :8080

# Run with Server-Sent Events
docker run -d \
  -p 8080:8080 \
  -v mcp-memory-data:/data \
  ghcr.io/jamesprial/mcp-memory-rewrite:latest -http :8080 -sse
```

### Use with Claude Desktop

Add this to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "memory": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "mcp-memory-data:/data",
        "ghcr.io/jamesprial/mcp-memory-rewrite:latest"
      ]
    }
  }
}
```

### Docker Compose

```yaml
version: '3.8'

services:
  mcp-memory:
    image: ghcr.io/jamesprial/mcp-memory-rewrite:latest
    ports:
      - "8080:8080"
    volumes:
      - memory-data:/data
    environment:
      - LOG_LEVEL=info
    command: ["-http", ":8080"]

volumes:
  memory-data:
```

### Building from Source

```bash
docker build -t mcp-memory-server .
docker run -i -v mcp-memory-data:/data mcp-memory-server
```

## CI & Benchmarks

- CI runs on GitHub Actions and includes:
  - Go build + unit tests
  - End-to-end tests across stdio, HTTP (streamable), and SSE
  - Benchmarks at multiple scales (configurable via `E2E_BENCH_SCALE`)

To run benchmarks locally with different scales:

```bash
pytest -q -m benchmark -s              # default scale 1
E2E_BENCH_SCALE=3 pytest -q -m benchmark -s
```

## Migration from JSON

If you have an existing JSON memory file from the original TypeScript implementation, you can migrate it using the included migration tool (to be implemented).

## Performance Comparison

| Feature | JSON Implementation | SQLite Implementation |
|---------|--------------------|-----------------------|
| Search Speed (1000 entities) | ~50ms | ~5ms |
| Write Speed (batch 100) | ~200ms | ~20ms |
| Memory Usage (10k entities) | ~100MB | ~20MB |
| Concurrent Access | Limited | Full support |
| Data Integrity | Basic | ACID compliant |
| Full-Text Search | String matching | FTS5 engine |

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This MCP server is licensed under the MIT License. This means you are free to use, modify, and distribute the software, subject to the terms and conditions of the MIT License.
