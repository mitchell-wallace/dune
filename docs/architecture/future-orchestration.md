# Future Orchestration

The Go module includes domain and task/event types that will support later orchestration work without introducing a daemon yet.

Near-term extension points:

- background task tracking for container operations
- agent/session lifecycle events
- local API or daemon split if dashboard work needs it

Out of scope for the current migration:

- daemon/service process
- HTTP API
- dashboard UI
