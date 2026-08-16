package telemetry

type HealthState string

const (
	HealthOK          HealthState = "ok"
	HealthNotDetected HealthState = "not_detected"
	HealthDegraded    HealthState = "degraded"
	HealthFailed      HealthState = "failed"
)

type SourceHealth struct {
	Source   SourceName
	State    HealthState
	Detail   string
	Indexing bool
}
