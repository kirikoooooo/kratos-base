package langfuse

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	datatrace "kratos-demo/internal/data/trace"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const maxObservedValue = 4096

type Observer struct {
	tracer   trace.Tracer
	provider *tracesdk.TracerProvider
	release  string
	log      *log.Helper
	mu       sync.Mutex
	tasks    map[string]taskSpan
}

type taskSpan struct {
	ctx  context.Context
	span trace.Span
}

type noopObserver struct{}

func New(config *conf.Observability, logger log.Logger) (datatrace.DelegationTraceObserver, func(), error) {
	cfg := config.GetLangfuse()
	if !cfg.GetEnabled() {
		return noopObserver{}, func() {}, nil
	}
	publicKey := os.Getenv(defaultEnv(cfg.GetPublicKeyEnv(), "LANGFUSE_PUBLIC_KEY"))
	secretKey := os.Getenv(defaultEnv(cfg.GetSecretKeyEnv(), "LANGFUSE_SECRET_KEY"))
	if strings.TrimSpace(publicKey) == "" || strings.TrimSpace(secretKey) == "" {
		log.NewHelper(logger).Warn("Langfuse is enabled but credentials are missing; external tracing is disabled")
		return noopObserver{}, func() {}, nil
	}

	// LANGFUSE_BASE_URL follows the official SDK convention and takes precedence
	// so deployments can switch Cloud/self-hosted endpoints without editing YAML.
	host := strings.TrimRight(defaultString(os.Getenv("LANGFUSE_BASE_URL"), defaultString(cfg.GetHost(), "https://cloud.langfuse.com")), "/")
	endpoint := host + "/api/public/otel/v1/traces"
	auth := base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
	helper := log.NewHelper(logger)
	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization":                "Basic " + auth,
			"x-langfuse-ingestion-version": "4",
		}),
	)
	if err != nil {
		helper.Warnf("initialize Langfuse exporter failed; external tracing is disabled: %v", err)
		return noopObserver{}, func() {}, nil
	}
	res, err := resource.New(context.Background(), resource.WithAttributes(
		attribute.String("service.name", "kratos-demo"),
		attribute.String("service.version", cfg.GetRelease()),
		attribute.String("deployment.environment.name", cfg.GetEnvironment()),
	))
	if err != nil {
		helper.Warnf("initialize Langfuse resource failed; external tracing is disabled: %v", err)
		return noopObserver{}, func() {}, nil
	}
	provider := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(loggingExporter{next: exporter, log: helper, endpoint: endpoint}),
		tracesdk.WithResource(res),
	)
	observer := &Observer{tracer: provider.Tracer("kratos-demo/agent"), provider: provider, release: cfg.GetRelease(), log: helper, tasks: make(map[string]taskSpan)}
	helper.Infof("Langfuse tracing enabled: endpoint=%s environment=%s release=%s", endpoint, cfg.GetEnvironment(), cfg.GetRelease())
	timeout := time.Duration(cfg.GetFlushTimeoutSeconds()) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return observer, func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := observer.Shutdown(ctx); err != nil {
			helper.Warnf("Langfuse shutdown/flush failed: %v", err)
		} else {
			helper.Info("Langfuse shutdown/flush completed")
		}
	}, nil
}

// loggingExporter reports each OTLP batch outcome without exposing headers or span payloads.
type loggingExporter struct {
	next     tracesdk.SpanExporter
	log      *log.Helper
	endpoint string
}

func (e loggingExporter) ExportSpans(ctx context.Context, spans []tracesdk.ReadOnlySpan) error {
	e.log.Infof("Langfuse OTLP export batch: endpoint=%s spans=%d", e.endpoint, len(spans))
	err := e.next.ExportSpans(ctx, spans)
	if err != nil {
		e.log.Warnf("Langfuse OTLP export batch failed: %v", err)
		return err
	}
	e.log.Infof("Langfuse OTLP export batch succeeded: spans=%d", len(spans))
	return nil
}

func (e loggingExporter) Shutdown(ctx context.Context) error { return e.next.Shutdown(ctx) }

func (o *Observer) StartTask(taskID string, agent public.AgentKind, status string) {
	if o == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if old, ok := o.tasks[taskID]; ok {
		old.span.SetAttributes(attribute.String("langfuse.trace.metadata.status", status))
		return
	}
	ctx, span := o.tracer.Start(context.Background(), "agent.task", trace.WithAttributes(
		attribute.String("langfuse.trace.name", "agent.task"),
		attribute.String("langfuse.session.id", taskID),
		attribute.String("langfuse.release", o.release),
		attribute.String("langfuse.trace.metadata.task_id", taskID),
		attribute.String("langfuse.trace.metadata.agent", string(agent)),
		attribute.String("langfuse.trace.metadata.status", status),
	))
	o.tasks[taskID] = taskSpan{ctx: ctx, span: span}
	o.log.Infof("Langfuse trace started: task_id=%s trace_id=%s agent=%s", taskID, span.SpanContext().TraceID(), agent)
}

func (o *Observer) UpdateTask(taskID, status string, result *taskv1.TaskResult, err error) {
	if o == nil {
		return
	}
	o.mu.Lock()
	task, ok := o.tasks[taskID]
	if ok {
		delete(o.tasks, taskID)
	}
	o.mu.Unlock()
	if !ok {
		return
	}
	attrs := []attribute.KeyValue{attribute.String("langfuse.trace.metadata.status", status)}
	if result != nil {
		attrs = append(attrs, attribute.String("langfuse.trace.output", limit(result.GetOutput())))
	}
	if err != nil {
		attrs = append(attrs,
			attribute.String("langfuse.observation.level", "ERROR"),
			attribute.String("langfuse.observation.status_message", limit(err.Error())),
		)
	}
	task.span.SetAttributes(attrs...)
	task.span.End()
	o.log.Infof("Langfuse trace ended: task_id=%s trace_id=%s status=%s", taskID, task.span.SpanContext().TraceID(), status)
}

func (o *Observer) AppendEvent(event datatrace.DelegationEvent) {
	if o == nil || strings.TrimSpace(event.TaskID) == "" {
		return
	}
	o.mu.Lock()
	task, ok := o.tasks[event.TaskID]
	o.mu.Unlock()
	if !ok {
		return
	}
	if event.Stage == "message_received" || event.Stage == "task_running" {
		task.span.SetAttributes(attribute.String("langfuse.trace.input", limit(event.PromptPreview)))
	}
	name, kind := eventName(event)
	timestamp := eventTime(event)
	_, span := o.tracer.Start(task.ctx, name,
		trace.WithTimestamp(timestamp),
		trace.WithAttributes(eventAttributes(event, kind)...),
	)
	span.End(trace.WithTimestamp(timestamp))
}

func (o *Observer) UpdatePlan(taskID string, steps []datatrace.PlanStep) {
	if o == nil {
		return
	}
	o.mu.Lock()
	task, ok := o.tasks[taskID]
	o.mu.Unlock()
	if ok {
		task.span.SetAttributes(attribute.Int("langfuse.trace.metadata.plan_steps", len(steps)))
	}
}

func (o *Observer) UpdateContextUsage(taskID string, usage datatrace.ContextUsageSnapshot, _ *datatrace.ContextCompressResult) {
	if o == nil {
		return
	}
	o.mu.Lock()
	task, ok := o.tasks[taskID]
	o.mu.Unlock()
	if ok {
		task.span.SetAttributes(attribute.Int("langfuse.trace.metadata.context_chars", usage.EstimatedChars))
	}
}

func (o *Observer) Shutdown(ctx context.Context) error {
	if o == nil || o.provider == nil {
		return nil
	}
	o.log.Info("Langfuse flushing queued spans")
	return o.provider.Shutdown(ctx)
}

func (noopObserver) StartTask(string, public.AgentKind, string)           {}
func (noopObserver) UpdateTask(string, string, *taskv1.TaskResult, error) {}
func (noopObserver) AppendEvent(datatrace.DelegationEvent)                {}
func (noopObserver) UpdatePlan(string, []datatrace.PlanStep)              {}
func (noopObserver) UpdateContextUsage(string, datatrace.ContextUsageSnapshot, *datatrace.ContextCompressResult) {
}
func (noopObserver) Shutdown(context.Context) error { return nil }

func eventName(event datatrace.DelegationEvent) (string, string) {
	if event.Stage == "llm_generate" {
		return "generate-response", "generation"
	}
	if event.ToolName != "" || strings.HasPrefix(event.Stage, "tool_") {
		return "tool." + defaultString(event.ToolName, strings.TrimPrefix(event.Stage, "tool_")), "tool"
	}
	if strings.HasPrefix(event.Stage, "delegate_") || strings.HasPrefix(event.Stage, "remote_") {
		return "agent.delegate", "delegation"
	}
	return "agent." + defaultString(event.Stage, "event"), "event"
}

func eventAttributes(event datatrace.DelegationEvent, kind string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("langfuse.observation.type", observationType(kind)),
		attribute.String("langfuse.observation.metadata.kind", kind),
		attribute.String("langfuse.observation.metadata.stage", event.Stage),
		attribute.String("langfuse.observation.metadata.agent", event.Agent),
		attribute.String("langfuse.observation.metadata.target", event.Target),
		attribute.String("langfuse.observation.input", limit(event.ToolInput)),
		attribute.String("langfuse.observation.output", limit(firstNonEmpty(event.ToolOutput, event.Summary))),
		attribute.Int64("langfuse.observation.metadata.duration_ms", event.DurationMS),
	}
	if kind == "generation" {
		attrs = append(attrs,
			attribute.String("langfuse.observation.model.name", event.Model),
			attribute.String("langfuse.observation.usage_details", fmt.Sprintf(`{"input":%d,"output":%d}`, event.InputTokens, event.OutputTokens)),
		)
	}
	if event.Error != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.level", "ERROR"), attribute.String("langfuse.observation.status_message", limit(event.Error)))
	}
	return attrs
}

func observationType(kind string) string {
	switch kind {
	case "tool", "generation", "event":
		return kind
	default:
		return "span"
	}
}

func eventTime(event datatrace.DelegationEvent) time.Time {
	if event.Time.IsZero() {
		return time.Now()
	}
	return event.Time
}
func defaultEnv(value, fallback string) string {
	return defaultString(strings.TrimSpace(value), fallback)
}
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func limit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxObservedValue {
		return value
	}
	return value[:maxObservedValue] + "..."
}
