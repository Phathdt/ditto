# Ditto

A service that helps implement the **Event-Driven architecture**.

To maintain the consistency of data in the system, we will use **transactional messaging** -
publishing events in a single transaction with a domain model change.

The service allows you to subscribe to changes in the PostgreSQL database using its logical decoding capability
and publish them to the NATS Streaming server.

## Logic of work
To receive events about data changes in our PostgreSQL DB
  we use the standard logic decoding module (**pgoutput**) This module converts
changes read from the WAL into a logical replication protocol.
  And we already consume all this information on our side.
Then we filter out only the events we need and publish them in the queue

### Event publishing

As the message broker will be used is of your choice:
NATS JetStream [`type=nats`]

Service publishes the following structure.
The name of the topic for subscription to receive messages is formed from the prefix of the topic,
the name of the database and the name of the table `prefix_schema_table`.

```go
{
	ID        uuid.UUID       # unique ID
	Schema    string
	Table     string
	Action    string
	Data      map[string]any
	DataOld   map[string]any  # old data (see DB-settings note #1)
	EventTime time.Time       # commit time
}
```

Messages are published to the broker at least once!

### Filter configuration example

```bash
TABLE_FILTER={"trades":["insert","update"]}
```
This filter means that we only process events occurring with the `trades` table,
and in particular `insert` and `update` data.

### Topic mapping
By default, output NATS topic name consist of prefix, DB schema, and DB table name,
but if you want to send all update in one topic you should be configured the topic map:
```bash
TOPIC_MAPPING={"trades":"transactions"}
```

## DB setting
You must make the following settings in the db configuration (postgresql.conf)
* wal_level >= “logical”
* max_replication_slots >= 1

The publication & slot created automatically when the service starts (for all tables and all actions).
You can delete the default publication and create your own (name: _Ditto_) with the necessary filtering conditions, and then the filtering will occur at the database level and not at the application level.

https://www.postgresql.org/docs/current/sql-createpublication.html

If you change the publication, do not forget to change the slot name or delete the current one.

Notes:

1. To receive `DataOld` field you need to change REPLICA IDENTITY to FULL as described here:
   [#SQL-ALTERTABLE-REPLICA-IDENTITY](https://www.postgresql.org/docs/current/sql-altertable.html#SQL-ALTERTABLE-REPLICA-IDENTITY)

## Service configuration
```bash
DB_DSN=postgres://postgres:password@localhost:5432/db_name?replication=database
## output plugin (-output_plugin)
OUTPUT_PLUGIN="pgoutput"

## publication name (-publication_name)
PUBLICATION_NAME=ditto

## slot name replica (-slot_name)
SLOT_NAME=ditto

NATS_PUB_URI="nats://localhost:4222"

TABLE_FILTER={"trades":["insert"]}

TOPIC_MAPPING={"trades":"transactions"}

```
## TODO
- [ ] update condition filter
- [ ] refactor code listener
- [ ] add more publisher
- [ ] move config to json api
- [ ] have server to config
- [ ] enable run in cluster
