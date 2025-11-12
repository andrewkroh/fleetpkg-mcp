# fleetpkg-mcp

`fleetpkg-mcp` is a Model Context Protocol (MCP) server that enables 
LLMs to query low-level metadata about Elastic Fleet integration packages.
It loads metadata from a local copy of the `elastic/integrations` repository
into a SQLite database and exposes SQL query capabilities to the LLM through the
Model Context Protocol.

Each time the MCP is started, it will rebuild the database. This takes about 10
seconds on a fast machine.

## Features

- Scans and indexes all Elastic Fleet integration packages from your local `elastic/integrations` repository
- Creates a queryable SQLite database with comprehensive package metadata
- Exposes readonly database access to LLMs through the Model Context Protocol
- Enables AI assistants to answer detailed questions about Elastic Fleet integrations

## Installation

### Install from source

```bash
go install github.com/andrewkroh/fleetpkg-mcp@latest
```

This will install the binary to your `$GOPATH/bin` directory (typically `~/go/bin`).

### Run without installing

You can also run the server directly without installing:

```bash
go run github.com/andrewkroh/fleetpkg-mcp@main -dir /path/to/integrations
```

## MCP Server Setup

The `fleetpkg-mcp` server can be configured as an MCP server in your AI
assistant. The server requires the `-dir` argument pointing to your local
checkout of the [elastic/integrations](https://github.com/elastic/integrations)
repository.

### Claude Desktop Setup

#### Using stdio transport (recommended)

Add the server using the Claude CLI:

```bash
claude mcp add --scope user fleetpkg -- /Users/<USERNAME>/go/bin/fleetpkg-mcp -dir /path/to/integrations
```

Or manually add to your Claude Desktop configuration file:

```json
{
  "mcpServers": {
    "fleetpkg": {
      "command": "/Users/<USERNAME>/go/bin/fleetpkg-mcp",
      "args": [
        "-dir",
        "/path/to/integrations"
      ]
    }
  }
}
```

To remove:

```bash
claude mcp remove fleetpkg
```

#### Using HTTP transport

First, start the server in HTTP mode:

```bash
fleetpkg-mcp -dir /path/to/integrations -http 127.0.0.1:1234
```

Then add the HTTP endpoint to Claude Desktop:

```bash
claude mcp add --scope user --transport http fleetpkg http://127.0.0.1:1234
```

### Other MCP Clients

For other MCP-compatible clients, use one of these configuration formats:

#### With installed binary

```json
{
  "mcpServers": {
    "fleetpkg": {
      "command": "/path/to/fleetpkg-mcp",
      "args": ["-dir", "/path/to/integrations"]
    }
  }
}
```

#### With go run

```json
{
  "mcpServers": {
    "fleetpkg": {
      "command": "go",
      "args": [
        "run",
        "github.com/andrewkroh/fleetpkg-mcp@main",
        "-dir",
        "/path/to/integrations"
      ]
    }
  }
}
```

## CLI Usage

The server can be run directly from the command line for testing or HTTP mode:

```bash
# Basic usage with stdio (for MCP)
fleetpkg-mcp -dir /path/to/integrations

# HTTP mode (for HTTP-based MCP clients)
fleetpkg-mcp -dir /path/to/integrations -http 127.0.0.1:1234

# With custom log level
fleetpkg-mcp -dir /path/to/integrations -log-level debug

# Disable logging
fleetpkg-mcp -dir /path/to/integrations -no-log

# Show version
fleetpkg-mcp -version
```

### Arguments

#### Required

- `-dir <path>`: Path to your local checkout of the [elastic/integrations](https://github.com/elastic/integrations) repository.

#### Optional

- `-http <address>`: Listen for HTTP connections at the specified address instead of using stdin/stdout. Example: `127.0.0.1:1234`
- `-log-level <level>`: Set log level. Options: `debug`, `info`, `warn`, `error`. Default: `info`
- `-no-log`: Disable all logging output
- `-version`: Print version information and exit

## Database Schema

The SQLite database contains information about Fleet integrations including:

- **Integrations**: Core metadata about each package (name, version, type, description, ownership)
- **Policy Templates**: Configuration templates for deploying integrations with deployment modes
- **Data Streams**: Information about the data streams each integration produces
- **Fields**: Detailed field definitions from fields.yml files with ECS mappings
- **Transforms**: Data transformation configurations with pivot and latest operations
- **Variables**: Configuration variables for customizing integrations with options for select types
- **Ingest Pipelines**: Elasticsearch ingest pipeline configurations
- **Ingest Processors**: Individual processors within pipelines including nested on_failure handlers
- **Sample Events**: Example event data for data streams
- **Icons and Screenshots**: Visual assets for integrations and policy templates with image metadata
- **Discovery Fields**: Package discovery capability metadata
- **Build Manifests**: Build configuration and ECS dependencies
- **Changelogs**: Version history with releases and individual changes
- **Categories**: Categorization for integrations and policy templates

For the complete database schema, see [schema.sql](internal/database/schema.sql).

## Example Conversation

Here's an example of how you can use this server with an LLM to answer questions about Elastic Fleet integrations:

> **Q**: Use the fleetpkg tools to answer: What integrations configure a pivot type transform?
>
> **A**: The following integrations configure a pivot type transform:
>
> * aws_billing
> * beaconing
> * ded
> * elasticsearch
> * lmd
> * pad

> **Q**: Of those, which do not have an ingest node pipeline associated with the transform?
>
> **A**: The integrations that configure a pivot type transform and do not have an ingest node pipeline associated with the transform are:
>
> * lmd
> * pad

> **Q**: What github team owns those two integrations?
>
> **A**: The `elastic/sec-applied-ml` GitHub team owns both the `lmd` and `pad` integrations.

----

> **Q**: Use the fleetpkg tools to answer the following question. What integration
> data streams define a 'resource' field? And what are the data types of the
> fields?
> 
> **A**: The following integration data streams define a 'resource' field of type 'keyword':
>
> - endace > flow
> - network_traffic > amqp
> - network_traffic > cassandra
> - network_traffic > dhcpv4
> - network_traffic > dns
> - network_traffic > flow
> - network_traffic > http
> - network_traffic > icmp
> - network_traffic > memcached
> - network_traffic > mongodb
> - network_traffic > mysql
> - network_traffic > nfs
> - network_traffic > pgsql
> - network_traffic > redis
> - network_traffic > sip
> - network_traffic > thrift
> - network_traffic > tls

----

> **Q**: Analyze the screenshots to determine the percentage of screenshots that
> have the correct 'size' column in comparison to the computed width and height
> column values. Include the total numbers as well.
>
> *A*: Based on my analysis of the screenshots in the database, here are the results:
>
> Summary Statistics
>
> - Total screenshots: 932
> - Correct size values: 169 (18.13%)
> - Incorrect size values: 763 (81.87%)
> 
> Analysis
> 
> Only 18.13% of screenshots have the correct 'size' column value that matches the computed dimensions from the width and height columns. This means that 81.87% of the screenshots have mismatched or incorrect size specifications.
> 
> Common Issues Found
> 
> Looking at the sample data, I can see several types of mismatches:
> 
> 1. Placeholder values: Many screenshots have size="600x600" when the actual dimensions are different (e.g., 1920x1080, 1899x2069, 1920x1233)
> 2. Scaled/resized values: Screenshots show evidence of being resized - the size column contains smaller dimensions than the actual image dimensions (e.g., size shows 1896x961 but actual is 2880x1800)
> 3. Character encoding issues: At least one case shows size="1608×2692" (using × instead of x) while computed is "1608x2692"
> 4. Minor width discrepancies: Some have small differences in width (e.g., size="1280x1329" vs computed "1268x1329")

----

> **Q**: Use the fleetpkg MCP tool. Analyze the number of 'set' ingest
> processors that use the 'copy_from' attribute vs the 'value' attribute with a
> Mustache template value matching '{{%}}'
>
> **A**:
> Here are the results of the analysis of 'set' ingest processors:
> - Number of processors using the `copy_from` attribute: 6992
> - Number of processors using the `value` attribute with a Mustache template:
    1174
>
> **Q**: What are the top 5 teams responsible for the packages using 'set' with
> a Mustache template instead of 'copy_from'?
> 
> **A**:
> Here are the top 5 teams using 'set' with a Mustache template instead of 'copy_from':
>
> 1. elastic/security-service-integrations: 395 uses
> 2. elastic/integration-experience: 321 uses
> 3. elastic/obs-infraobs-integrations: 245 uses
> 4. elastic/obs-ds-hosted-services: 100 uses
> 5. elastic/elastic-agent-data-plane: 36 uses

## License

This project is licensed under the Apache License 2.0 - see the LICENSE.txt file for details.
