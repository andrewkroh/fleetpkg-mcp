# fleetpkg-mcp

`fleetpkg-mcp` is a Model Context Protocol (MCP) server that enables 
LLMs to query low-level metadata about Elastic Fleet integration packages.
It loads metadata from a local copy of the `elastic/integrations` repository
into a SQLite database and exposes SQL query capabilities to the LLM through the
Model Context Protocol.

## Features

- Scans and indexes all Elastic Fleet integration packages from your local `elastic/integrations` repository
- Creates a queryable SQLite database with comprehensive package metadata
- Exposes readonly database access to LLMs through the Model Context Protocol
- Enables AI assistants to answer detailed questions about Elastic Fleet integrations

## Usage

### With `go run`

```
{
  "mcpServers": {
    "fleetpkg": {
      "command": "go",
      "args": [
        "run",
        "github.com/andrewkroh/fleetpkg-mcp@main",
        "-dir"
        "/Users/<USERNAME>/code/elastic/integrations"
      ]
    }
  }
}
```

### Local install

Install the binary with

`go install github.com/andrewkroh/fleetpkg-mcp`

then determine the path using `which fleetpkg-mcp`.

```
{
  "mcpServers": {
    "fleetpkg": {
      "command": "/Users/<USERNAME>/go/bin/fleetpkg-mcp"
    }
  }
}
```

### Required Arguments

- `-dir`: **Required**. Path to your local checkout of the [elastic/integrations](https://github.com/elastic/integrations) repository.

### Optional Arguments

- `-http`: Listen for HTTP at the specified address, instead of using stdin/stdout
- `-log-level`: Set log level (debug, info, warn, error). Default: info
- `-no-log`: Disable logging

## Database Schema

The SQLite database contains information about Fleet integrations including:

- **Integrations**: Core metadata about each package (name, version, type, description, ownership)
- **Policy Templates**: Configuration templates for deploying integrations
- **Data Streams**: Information about the data streams each integration produces
- **Fields**: Detailed field definitions from fields.yml files
- **Transforms**: Data transformation configurations
- **Variables**: Configuration variables for customizing integrations

There is more data to expose, but this is an MVP.

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

## License

This project is licensed under the Apache License 2.0 - see the LICENSE.txt file for details.
