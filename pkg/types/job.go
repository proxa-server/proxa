package types

import "time"

// Job is a one-shot or cron workload. Spec.Schedule controls cadence
// (empty = run-once); LastRun and NextRunAt are bookkeeping fields
// updated by the scheduler.
type Job struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Name      string    `json:"name"`
	Spec      TaskDef   `json:"spec"`
	LastRun   *JobRun   `json:"lastRun,omitempty"`
	NextRunAt time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// JobRun is one execution of a job. LogsRef is an opaque pointer
// resolvable by whatever log storage the runtime uses.
type JobRun struct {
	StartedAt time.Time     `json:"startedAt"`
	EndedAt   time.Time     `json:"endedAt,omitempty"`
	Duration  time.Duration `json:"duration"`
	ExitCode  int           `json:"exitCode"`
	Status    string        `json:"status"`  // running | succeeded | failed | timeout
	LogsRef   string        `json:"logsRef"`
}
