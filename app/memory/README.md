# Memory Service

Owns the read-side prompt-context API for shared Agent memory. It serves `api.memory.v1.MemoryService/GetPromptContext` over gRPC, defaulting to `127.0.0.1:9002` (`MEMORY_GRPC_ADDR` overrides it).

Conversation mutation, error recording and compression remain in `agent-runtime` until a transactional write contract is agreed. This prevents two services from concurrently rewriting one session's conversation history.
