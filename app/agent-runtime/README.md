# Agent Runtime Service

Owns the authoritative A2A Task lifecycle, agent execution, tool calls, and model access.

Entrypoint: `app/agent-runtime/cmd`. Its internal gRPC task endpoint defaults to `127.0.0.1:9000` and can be changed with `KRATOS_INTERNAL_GRPC_ADDR`.

Public interoperability remains A2A JSON-RPC; internal callers use gRPC.
