package telemetry

// Trace attributes consistent with Langfuse/OTEL standards.
// These are used to tag spans for better filtering and visualization in observability UIs.

const (
	// Trace level attributes
	AttrTraceName      = "langfuse.trace.name"
	AttrTraceUserID    = "langfuse.user.id"
	AttrTraceSessionID = "langfuse.session.id"
	AttrTraceTags      = "langfuse.trace.tags"
	AttrTracePublic    = "langfuse.trace.public"
	AttrTraceMetadata  = "langfuse.trace.metadata"
	AttrTraceInput     = "langfuse.trace.input"
	AttrTraceOutput    = "langfuse.trace.output"

	// Observation level attributes
	AttrObservationType         = "langfuse.observation.type"
	AttrObservationMetadata     = "langfuse.observation.metadata"
	AttrObservationLevel        = "langfuse.observation.level"
	AttrObservationInput        = "langfuse.observation.input"
	AttrObservationOutput       = "langfuse.observation.output"
	AttrObservationModel        = "langfuse.observation.model.name"
	AttrObservationUsageDetails = "langfuse.observation.usage_details"
)

// Standard observation types
const (
	ObservationTypeEvent      = "event"
	ObservationTypeSpan       = "span"
	ObservationTypeGeneration = "generation"
	ObservationTypeAgent      = "agent"
	ObservationTypeTool       = "tool"
	ObservationTypeChain      = "chain"
	ObservationTypeRetriever  = "retriever"
)

// Attribute creation helpers
func StringAttr(key string, value string) Attribute {
	return Attribute{Key: key, Value: value}
}

func IntAttr(key string, value int) Attribute {
	return Attribute{Key: key, Value: value}
}

func BoolAttr(key string, value bool) Attribute {
	return Attribute{Key: key, Value: value}
}
