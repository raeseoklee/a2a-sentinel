package i18n

// MCP tool description keys.
const (
	ToolListAgentsDesc         = "tool.list_agents.desc"
	ToolHealthCheckDesc        = "tool.health_check.desc"
	ToolGetBlockedRequestsDesc = "tool.get_blocked_requests.desc"
	ToolGetAgentCardDesc       = "tool.get_agent_card.desc"
	ToolGetAggregatedCardDesc  = "tool.get_aggregated_card.desc"
	ToolGetRateLimitStatusDesc = "tool.get_rate_limit_status.desc"
	ToolUpdateRateLimitDesc    = "tool.update_rate_limit.desc"
	ToolRegisterAgentDesc      = "tool.register_agent.desc"
	ToolDeregisterAgentDesc    = "tool.deregister_agent.desc"
	ToolSendTestMessageDesc    = "tool.send_test_message.desc"
	ToolListPoliciesDesc       = "tool.list_policies.desc"
	ToolEvaluatePolicyDesc     = "tool.evaluate_policy.desc"
	ToolListPendingChangesDesc = "tool.list_pending_changes.desc"
	ToolApproveCardChangeDesc  = "tool.approve_card_change.desc"
	ToolRejectCardChangeDesc   = "tool.reject_card_change.desc"
)

// MCP tool parameter description keys.
const (
	ParamSinceDesc         = "param.since.desc"
	ParamLimitDesc         = "param.limit.desc"
	ParamAgentNameDesc     = "param.agent_name.desc"
	ParamPerMinuteDesc     = "param.per_minute.desc"
	ParamNameDesc          = "param.name.desc"
	ParamURLDesc           = "param.url.desc"
	ParamDefaultDesc       = "param.default.desc"
	ParamTextDesc          = "param.text.desc"
	ParamAgentDesc         = "param.agent.desc"
	ParamMethodDesc        = "param.method.desc"
	ParamUserDesc          = "param.user.desc"
	ParamIPDesc            = "param.ip.desc"
	ParamAgentNameCardDesc = "param.agent_name_card.desc"
)

// MCP resource keys.
const (
	ResConfigName      = "res.config.name"
	ResConfigDesc      = "res.config.desc"
	ResMetricsName     = "res.metrics.name"
	ResMetricsDesc     = "res.metrics.desc"
	ResAgentDetailName = "res.agent_detail.name"
	ResAgentDetailDesc = "res.agent_detail.desc"
	ResSecurityName    = "res.security.name"
	ResSecurityDesc    = "res.security.desc"
)

// MCP server error keys.
const (
	ErrMethodNotAllowed         = "err.method_not_allowed"
	ErrUnauthorizedInvalidToken = "err.unauthorized_invalid_token"
	ErrParseError               = "err.parse_error"
	ErrSessionRequired          = "err.session_required"
	ErrSessionNotFound          = "err.session_not_found"
	ErrInvalidParams            = "err.invalid_params"
	ErrMethodNotFound           = "err.method_not_found"
	ErrUnknownTool              = "err.unknown_tool"
	ErrInternalError            = "err.internal_error"
	ErrWriteTokenNotConfigured  = "err.write_token_not_configured"
	ErrWriteTokenRequired       = "err.write_token_required"
	ErrAgentNameRequired        = "err.agent_name_required"
	ErrNameRequired             = "err.name_required"
	ErrURLRequired              = "err.url_required"
	ErrTextRequired             = "err.text_required"
	ErrPerMinutePositive        = "err.per_minute_positive"
	ErrInvalidSinceTimestamp    = "err.invalid_since_timestamp"
	ErrParsingArguments         = "err.parsing_arguments"
	ErrUnknownResourceURI       = "err.unknown_resource_uri"
	ErrAgentNameRequiredInURI   = "err.agent_name_required_in_uri"
)

// SentinelError keys (gateway-level errors).
const (
	SentinelErrAuthRequiredMsg      = "sentinel.auth_required.msg"
	SentinelErrAuthRequiredHint     = "sentinel.auth_required.hint"
	SentinelErrAuthInvalidMsg       = "sentinel.auth_invalid.msg"
	SentinelErrAuthInvalidHint      = "sentinel.auth_invalid.hint"
	SentinelErrForbiddenMsg         = "sentinel.forbidden.msg"
	SentinelErrForbiddenHint        = "sentinel.forbidden.hint"
	SentinelErrRateLimitedMsg       = "sentinel.rate_limited.msg"
	SentinelErrRateLimitedHint      = "sentinel.rate_limited.hint"
	SentinelErrAgentUnavailableMsg  = "sentinel.agent_unavailable.msg"
	SentinelErrAgentUnavailableHint = "sentinel.agent_unavailable.hint"
	SentinelErrStreamLimitMsg       = "sentinel.stream_limit.msg"
	SentinelErrStreamLimitHint      = "sentinel.stream_limit.hint"
	SentinelErrReplayMsg            = "sentinel.replay.msg"
	SentinelErrReplayHint           = "sentinel.replay.hint"
	SentinelErrSSRFMsg              = "sentinel.ssrf.msg"
	SentinelErrSSRFHint             = "sentinel.ssrf.hint"
	SentinelErrInvalidRequestMsg    = "sentinel.invalid_request.msg"
	SentinelErrInvalidRequestHint   = "sentinel.invalid_request.hint"
	SentinelErrNoRouteMsg           = "sentinel.no_route.msg"
	SentinelErrNoRouteHint          = "sentinel.no_route.hint"
	SentinelErrGlobalLimitMsg       = "sentinel.global_limit.msg"
	SentinelErrGlobalLimitHint      = "sentinel.global_limit.hint"
)

// Health check keys.
const (
	HealthOK       = "health.ok"
	HealthReady    = "health.ready"
	HealthNotReady = "health.not_ready"
)

// Audit log value keys.
const (
	AuditAllowed = "audit.allowed"
	AuditBlocked = "audit.blocked"
)
