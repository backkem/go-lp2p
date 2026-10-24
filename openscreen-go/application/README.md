# Application Protocol

[W3C Specification](https://www.w3.org/TR/openscreen-application/)

Minimal OpenScreen Application Protocol implementation. This repo focuses on the network protocol layer; the application layer is kept intentionally sparse.

Currently implements AgentInfo message exchange for interop testing, and the
Presentation Protocol message set (`presentation.go`: type keys and CDDL-accurate
structs, no presentation state machine) plus presentation-id helpers.
