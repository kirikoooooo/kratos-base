# Dashboard Service

Owns the control plane: trace projections, Dashboard HTTP endpoints, SSE streams, and human approval UX.

It must not execute agents or become a second source of task state. During the first migration phase its packages remain wired into `app/agent-runtime/cmd`.
