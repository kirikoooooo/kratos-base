// Package errconst 集中管理项目中的所有哨兵错误值。
package errconst

import "errors"

// ---- agent_runtime ----

var (
	ErrAgentRuntimeUnavailable = errors.New("agent runtime is not available")
	ErrAgentNotSupported       = errors.New("agent is not supported")
	ErrDelegationNotSupported  = errors.New("delegation verification is not supported")
)

// ---- chat ----

var ErrChatClientNotAvailable = errors.New("chat client is not available")

// ---- provider ----

var ErrProviderNotAvailable = errors.New("chat client provider is not available")

// ---- tasking ----

var (
	ErrPromptRequired            = errors.New("prompt is required")
	ErrTaskNotFound              = errors.New("task not found")
	ErrTaskDispatcherUnavailable = errors.New("task dispatcher is unavailable")
)

// ---- tool ----

var ErrRiskApprovalDenied = errors.New("operation denied by user")

// ---- actor ----

var (
	ErrActorTimeout         = errors.New("time out")
	ErrActorMailOverflow    = errors.New("mail over flow")
	ErrActorSyncRequestSelf = errors.New("sync request self")
)
