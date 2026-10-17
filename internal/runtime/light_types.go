package runtime

import "github.com/stenhigh/prifly/internal/flow"

// Light is an observer contract. Its phases never become Run states.
type LightPhase struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Mapped     bool   `json:"mapped"`
	MappingRef string `json:"mapping_ref"`
}
type LightRun struct {
	BranchCount  int           `json:"branch_count"`
	ReasonCount  int           `json:"reason_count"`
	Truncated    bool          `json:"truncated"`
	ID           string        `json:"run_id"`
	Version      int64         `json:"run_version"`
	Events       int64         `json:"event_sequence"`
	Workflow     flow.Ref      `json:"workflow_ref"`
	TaskRevision *int64        `json:"task_revision"`
	Status       string        `json:"status"`
	Outcome      *string       `json:"outcome"`
	Parent       string        `json:"parent_run_id"`
	LastObserved string        `json:"last_observed"`
	Phases       []LightPhase  `json:"phases"`
	Board        *MonitorBoard `json:"board"`
	Reviews      []LightReview `json:"reviews"`
}
type LightCard struct {
	Key             string            `json:"key"`
	Kind            string            `json:"kind"`
	TaskID          *string           `json:"task_id"`
	TaskRevision    *int64            `json:"task_revision"`
	AuthorityID     string            `json:"authority_id"`
	ProjectID       string            `json:"project_id"`
	ProjectTitle    string            `json:"project_title"`
	Title           string            `json:"title"`
	TitleOrigin     string            `json:"title_origin"`
	QueueState      *string           `json:"queue_state"`
	Lane            string            `json:"lane"`
	Phase           LightPhase        `json:"phase"`
	Status          string            `json:"status"`
	Outcome         *string           `json:"outcome"`
	CompletionScope string            `json:"completion_scope"`
	Reasons         []MonitorReason   `json:"reasons"`
	Round           *int64            `json:"round"`
	CandidateDigest *string           `json:"candidate_digest"`
	CurrentRuns     []LightRun        `json:"current_runs"`
	CurrentCount    int               `json:"current_count"`
	ActiveBranches  []MonitorStage    `json:"active_branches"`
	BranchCount     int               `json:"branch_count"`
	Reviews         []LightReviewChip `json:"reviews"`
	ReviewCount     int               `json:"review_count"`
	HistoryCount    int               `json:"history_count"`
	Truncated       bool              `json:"truncated"`
	Priority        int64             `json:"priority"`
	Ordinal         int64             `json:"ordinal"`
	LastObserved    string            `json:"last_observed"`
}
type LightReview struct {
	ProducerVerdict string       `json:"producer_verdict"`
	ID              string       `json:"id"`
	RunID           string       `json:"run_id"`
	InvocationID    string       `json:"workflow_invocation_id"`
	ActivationID    string       `json:"stage_activation_id"`
	AttemptID       string       `json:"attempt_id"`
	IterationID     string       `json:"iteration_id"`
	Lens            string       `json:"lens"`
	Applicability   string       `json:"applicability"`
	Round           *int64       `json:"round"`
	CandidateDigest *string      `json:"candidate_digest"`
	RequestDigest   *string      `json:"request_digest"`
	Report          *ArtifactRef `json:"report_ref"`
	Gate            *ArtifactRef `json:"gate_ref"`
	Candidate       *ArtifactRef `json:"candidate_ref"`
	Request         *ArtifactRef `json:"request_ref"`
	ReportSchema    flow.Ref     `json:"report_schema"`
	GateSchema      flow.Ref     `json:"gate_schema"`
	GateScope       string       `json:"gate_scope"`
	CheckStatus     string       `json:"check_status"`
	GateStatus      string       `json:"gate_status"`
	Blocking        bool         `json:"blocking"`
	OwnerOnly       bool         `json:"blocking_owner_only"`
	Freshness       string       `json:"freshness"`
	State           string       `json:"state"`
	Findings        []string     `json:"findings"`
	FindingCount    int          `json:"finding_count"`
	Truncated       bool         `json:"truncated"`
	Reason          string       `json:"reason"`
	MappingDigest   string       `json:"mapping_digest"`
	Cut             int64        `json:"cut"`
}
type LightSourceView struct {
	Cut           int64
	Cards         []LightCard
	Runs          map[string][]LightRun
	Reviews       map[string][]LightReview
	Decodes       int
	ArtifactReads int
	QueueDecodes  int
}

type LightReviewChip struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	Lens        string `json:"lens"`
	State       string `json:"state"`
	CheckStatus string `json:"check_status"`
	GateStatus  string `json:"gate_status"`
	Freshness   string `json:"freshness"`
	GateScope   string `json:"gate_scope"`
	Reason      string `json:"reason"`
}
type LightSourceInfo struct {
	Source        string  `json:"source"`
	AuthorityID   string  `json:"authority_id"`
	ProjectID     string  `json:"project_id"`
	Cut           int64   `json:"cut"`
	ObservedAt    string  `json:"observed_at"`
	LastSuccessAt string  `json:"last_success_at"`
	Freshness     string  `json:"freshness"`
	Coverage      string  `json:"coverage"`
	Error         *string `json:"error"`
}
type LightPage struct {
	Limit      int     `json:"limit"`
	NextCursor *string `json:"next_cursor"`
	Total      *int    `json:"total"`
	Complete   bool    `json:"complete"`
}
type LightBoardCard struct {
	Source string `json:"source"`
	LightCard
}
type LightBoardPage struct {
	SchemaVersion  string            `json:"schema_version"`
	MappingDigest  *string           `json:"mapping_digest"`
	Generation     string            `json:"generation"`
	Sources        []LightSourceInfo `json:"sources"`
	Cards          []LightBoardCard  `json:"cards"`
	Page           LightPage         `json:"page"`
	Counters       map[string]int    `json:"counters"`
	FullMonitorURL string            `json:"full_monitor_url"`
}
type LightTaskPage struct {
	FullMonitorURL string          `json:"full_monitor_url"`
	SchemaVersion  string          `json:"schema_version"`
	Source         LightSourceInfo `json:"source"`
	Card           LightCard       `json:"card"`
	Runs           []LightRun      `json:"runs"`
	Page           LightPage       `json:"page"`
}
type LightReviewsPage struct {
	SchemaVersion string          `json:"schema_version"`
	Source        LightSourceInfo `json:"source"`
	Reviews       []LightReview   `json:"reviews"`
	Page          LightPage       `json:"page"`
}
type LightReportView struct {
	SchemaVersion string      `json:"schema_version"`
	Source        string      `json:"source"`
	RunID         string      `json:"run_id"`
	ReviewID      string      `json:"review_id"`
	Kind          string      `json:"kind"`
	Ref           ArtifactRef `json:"ref"`
	TotalBytes    int64       `json:"total_bytes"`
	Truncated     bool        `json:"truncated"`
	Text          string      `json:"text"`
}
