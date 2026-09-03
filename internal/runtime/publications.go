package runtime

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	// A maximal close carries 1024 ASCII identifiers plus its command envelope.
	maxPublishBodyBytes = 256 << 10
	maxPublicationBytes = 1 << 20 // Optional records cannot consume settlement reserve.
	stepReadVersion     = "foundation-step-read/1"
)

func validatePublishCommand(c PublishCommand) error {
	if c.SchemaVersion != "1" && c.SchemaVersion != "2" && c.SchemaVersion != "3" {
		return local.Reject("invalid_publication", "publication schema version is unsupported")
	}
	for _, id := range []string{c.CommandID, c.RunID, c.StepID, c.AttemptID} {
		b, _ := json.Marshal(id)
		if err := flow.ValidateProtocol("Identifier", b); err != nil {
			return local.Reject("invalid_publication", "publication identities are invalid")
		}
	}
	for name, value := range map[string]string{"PortName": c.Hook, "Digest": c.EnvelopeDigest} {
		b, _ := json.Marshal(value)
		if err := flow.ValidateProtocol(name, b); err != nil {
			return local.Reject("invalid_publication", "hook or envelope digest is invalid")
		}
	}
	switch c.Kind {
	case "state":
		if len(c.Value) == 0 || len(c.Value) > 65536 || c.ExpectedStateVersion == nil || *c.ExpectedStateVersion < 0 || *c.ExpectedStateVersion > (1<<53)-1 || c.EventKey != "" || c.ItemKey != "" || c.CandidatePath != "" || c.ExpectedDigest != "" || c.ExpectedSizeBytes != nil || c.ItemKeys != nil {
			return local.Reject("invalid_publication", "state requires an exact expected_state_version and no event_key")
		}
	case "event":
		b, _ := json.Marshal(c.EventKey)
		if len(c.Value) == 0 || len(c.Value) > 65536 || c.ExpectedStateVersion != nil || flow.ValidateProtocol("Identifier", b) != nil || c.ItemKey != "" || c.CandidatePath != "" || c.ExpectedDigest != "" || c.ExpectedSizeBytes != nil || c.ItemKeys != nil {
			return local.Reject("invalid_publication", "event requires a stable event_key and no state version")
		}
	case "artifact":
		if c.SchemaVersion != "2" && c.SchemaVersion != "3" {
			return local.Reject("unsupported_publication", "artifact publication requires the version 2 or 3 command contract")
		}
		digest, _ := json.Marshal(c.ExpectedDigest)
		if len(c.Value) != 0 || c.ExpectedStateVersion != nil || c.EventKey != "" || len(c.CandidatePath) > 4096 || !safeRelative(c.CandidatePath) || flow.ValidateProtocol("Digest", digest) != nil || c.ExpectedSizeBytes == nil || *c.ExpectedSizeBytes < 0 || *c.ExpectedSizeBytes > MaxArtifactBytes || c.ItemKeys != nil {
			return local.Reject("invalid_publication", "artifact requires a safe candidate path and exact bounded digest and size")
		}
		if c.ItemKey != "" {
			item, _ := json.Marshal(c.ItemKey)
			if flow.ValidateProtocol("Identifier", item) != nil {
				return local.Reject("invalid_publication", "artifact item_key is invalid")
			}
		}
	case "close":
		if c.SchemaVersion != "3" || c.ItemKeys == nil || len(c.ItemKeys) > MaxRunPublications || len(c.Value) != 0 || c.ExpectedStateVersion != nil || c.EventKey != "" || c.ItemKey != "" || c.CandidatePath != "" || c.ExpectedDigest != "" || c.ExpectedSizeBytes != nil {
			return local.Reject("invalid_publication", "close requires an explicit bounded item_keys list and no payload fields")
		}
		seen := map[string]bool{}
		for _, item := range c.ItemKeys {
			value, _ := json.Marshal(item)
			if flow.ValidateProtocol("Identifier", value) != nil || seen[item] {
				return local.Reject("invalid_publication", "close item_keys must be unique identifiers")
			}
			seen[item] = true
		}
	default:
		return local.Reject("unsupported_publication", "publication kind is unsupported")
	}
	return nil
}

// ParsePublishCommand preserves field presence so null or cross-variant fields
// cannot disappear into Go zero values at a wire boundary.
func ParsePublishCommand(data []byte) (PublishCommand, error) {
	var command PublishCommand
	if err := decode(data, &command); err != nil {
		return command, err
	}
	// Go's nil pointer/empty string cannot distinguish absent from JSON null
	// or an explicitly empty forbidden field. The wire variants remain closed.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return command, err
	}
	_, stateVersion := fields["expected_state_version"]
	_, eventKey := fields["event_key"]
	_, value := fields["value"]
	_, itemKeys := fields["item_keys"]
	artifactFields := []string{"item_key", "candidate_path", "expected_digest", "expected_size_bytes"}
	artifactPresent := false
	for _, name := range artifactFields {
		_, artifactPresent = fields[name]
		if artifactPresent {
			break
		}
	}
	if (command.Kind == "state" && (eventKey || artifactPresent || itemKeys)) || (command.Kind == "event" && (stateVersion || artifactPresent || itemKeys)) || command.Kind == "artifact" && (stateVersion || eventKey || value || itemKeys) || command.Kind == "close" && (stateVersion || eventKey || value || artifactPresent) {
		return command, local.Reject("invalid_publication", "fields from another publication variant are forbidden")
	}
	if command.Kind == "artifact" {
		for _, name := range []string{"candidate_path", "expected_digest", "expected_size_bytes"} {
			if _, exists := fields[name]; !exists {
				return command, local.Reject("invalid_publication", "artifact candidate identity is incomplete")
			}
		}
		if _, exists := fields["item_key"]; exists && command.ItemKey == "" {
			return command, local.Reject("invalid_publication", "an explicit item_key cannot be empty or null")
		}
	}
	if command.Kind == "close" && !itemKeys {
		return command, local.Reject("invalid_publication", "close item_keys is required, including for an empty stream")
	}
	return command, nil
}

// A retained token can read its own receipt after settlement, but never grants
// authority over a sibling. Clearing/replacing the hash revokes even that read.
func publicationAttempt(r Run, c PublishCommand) (*Attempt, *Activation, error) {
	denied := local.Reject("publisher_forbidden", "publisher has no current access to this namespace")
	a := r.Attempts[c.AttemptID]
	if a == nil || r.ID != c.RunID || a.StepID != c.StepID || a.EnvelopeDigest != c.EnvelopeDigest {
		return nil, nil, denied
	}
	s := r.Steps[a.StepID]
	activation := r.Activations[a.ActivationID]
	if s == nil || activation == nil || s.ActivationID != activation.ID || activation.StepID != s.ID || !slices.Contains(s.AttemptIDs, a.ID) {
		return nil, nil, denied
	}
	if isInvocationState(r.SchemaVersion) {
		invocation := r.Invocations[activation.InvocationID]
		if invocation == nil || invocation.ID != activation.InvocationID || invocation.RunID != r.ID || activation.Kind != "step" {
			return nil, nil, denied
		}
	}
	return a, activation, nil
}

func publisherAttempt(r Run, token string, c PublishCommand) (*Attempt, *Activation, error) {
	denied := local.Reject("publisher_forbidden", "publisher has no current access to this namespace")
	if len(token) != 64 {
		return nil, nil, denied
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, nil, denied
	}
	a := r.Attempts[c.AttemptID]
	if a == nil || subtle.ConstantTimeCompare([]byte(a.TokenHash), []byte(rawDigest([]byte(token)))) != 1 {
		return nil, nil, denied
	}
	return publicationAttempt(r, c)
}

// Publish is an authenticated fact intake, not a command to change lifecycle,
// produce an ArtifactRevision, dispatch another step or grant a permission.
func (e *Engine) Publish(ctx context.Context, token string, c PublishCommand) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if err := validatePublishCommand(c); err != nil {
		return local.ApplyResult{}, err
	}
	if c.Kind == "artifact" {
		return e.publishArtifact(ctx, token, c)
	}
	if c.Kind == "close" {
		return e.publishArtifactClosure(ctx, token, c)
	}
	value, err := flow.Canonical(c.Value)
	if err != nil {
		return local.ApplyResult{}, local.Reject("invalid_publication", "value must be strict bounded JSON")
	}
	c.Value = value
	r, _, err := e.load(ctx, c.RunID)
	if err != nil {
		return local.ApplyResult{}, local.Reject("publisher_forbidden", "publisher has no current access to this namespace")
	}
	a, activation, err := publisherAttempt(r, token, c)
	if err != nil {
		return local.ApplyResult{}, err
	}
	plan, err := r.planFor(activation.InvocationID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	definition := plan.Steps[activation.StageID]
	definitionRef := r.Steps[a.StepID].Ref
	hook, invalid := plan.ValidatePublication(activation.StageID, c.Hook, c.Kind, value)
	actor := "publisher:" + a.ID
	digest := rawDigest(value)
	result, err := e.apply(ctx, actor, c.CommandID, c.RunID, "step.publication", c, nil, local.CommandPublication, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current, currentActivation, err := currentPublisher(*r, token, c, definitionRef, activation.InvocationID, activation.StageID, obs)
		if err != nil {
			return local.Change{}, err
		}
		if invalid != nil {
			var p *flow.Problem
			if errors.As(invalid, &p) {
				return local.Change{}, local.Reject(p.Code, p.Message)
			}
			return local.Change{}, local.Reject("invalid_publication", "value does not satisfy the declared hook contract")
		}
		if (r.restrictedFor(currentActivation.InvocationID) || r.CancelRequested || current.Status == "stopping") && !hook.AllowDuringStop {
			return local.Change{}, local.Reject("publication_restricted", "this hook is not allowed during a stop")
		}
		var version, count, recent int64
		for _, old := range r.Publications {
			if old.StepID == c.StepID && old.Hook == c.Hook && c.Kind == "event" && old.Kind == "event" && old.EventKey == c.EventKey {
				if old.Digest != digest {
					return local.Change{}, local.Reject("event_key_conflict", "event_key already identifies a different payload")
				}
				result, err := canonical(map[string]any{"publication": old})
				return local.Change{ReceiptOnly: true, Result: result}, err
			}
			if old.AttemptID != c.AttemptID || old.Hook != c.Hook {
				continue
			}
			count++
			if old.Kind == "state" && old.Version > version {
				version = old.Version
			}
			if old.Received.Session != obs.Session || old.Received.MonotonicMS > obs.MonotonicMS {
				return local.Change{}, local.Reject("publisher_clock_unknown", "receipt order cannot be established for this publisher generation")
			}
			if obs.MonotonicMS-old.Received.MonotonicMS < 60000 {
				recent++
			}
		}
		if len(r.Publications) >= MaxRunPublications || count >= hook.MaxCount {
			return local.Change{}, local.Reject("publication_count_exhausted", "declared publication count limit reached")
		}
		if recent >= hook.MaxPerMinute {
			return local.Change{}, local.Reject("publication_rate_exhausted", "declared publication rate limit reached")
		}
		if c.Kind == "state" {
			if *c.ExpectedStateVersion != version {
				return local.Change{}, local.Reject("state_version_conflict", "expected state version differs from this Attempt's hook version")
			}
			if err := checkCounterProgress(r.Publications, definition.Telemetry, c); err != nil {
				return local.Change{}, err
			}
			version++
		}
		pub := Publication{ID: derivedID("publication", actor, c.CommandID), AttemptID: c.AttemptID, StepID: c.StepID, Hook: c.Hook, Kind: c.Kind, Version: version, EventKey: c.EventKey, Value: value, Digest: digest, Received: obs, Actor: actor}
		if c.Kind == "event" {
			pub.ID = derivedID("publication", r.ID, c.StepID, c.Hook, c.EventKey)
		}
		r.Publications = append(r.Publications, pub)
		for _, mapping := range definition.Telemetry {
			if mapping.Hook == c.Hook && mapping.Kind == "diagnostic" {
				if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", pub.ID, mapping.Name, mapping.Revision), RunID: r.ID, AttemptID: c.AttemptID, Origin: "worker-reported", Severity: mapping.Severity, Code: mapping.Code, Category: "application", Phase: "publication", Message: mapping.Message, Observed: obs, PublicationID: pub.ID, CauseRefs: []string{pub.ID}}); err != nil {
					return local.Change{}, err
				}
			}
		}
		var mapped []Diagnostic
		for _, d := range r.Diagnostics {
			if d.PublicationID != "" {
				mapped = append(mapped, d)
			}
		}
		optional, err := canonicalState(map[string]any{"publications": r.Publications, "diagnostics": mapped})
		if err != nil {
			return local.Change{}, err
		}
		if len(optional) > maxPublicationBytes {
			return local.Change{}, local.Reject("publication_budget_exhausted", "optional publication budget is exhausted; control reserve remains available")
		}
		// The event is metadata only. Payload is a classified own-namespace value
		// in the projection; a log line never becomes a trusted diagnostic.
		event, err := canonical(map[string]any{"publication_id": pub.ID, "step_instance_id": c.StepID, "attempt_id": c.AttemptID, "hook": c.Hook, "kind": c.Kind, "state_version": version, "event_key": c.EventKey, "digest": digest, "observation": obs, "origin": "worker-reported", "trust": "worker-reported"})
		if err != nil {
			return local.Change{}, err
		}
		result, err := canonical(map[string]any{"publication": pub})
		return local.Change{Events: []local.EventInput{{Type: "step.publication", Version: 1, Data: event}}, Result: result}, err
	})
	return e.publisherReceipt(ctx, token, c, result, err)
}

func currentPublisher(r Run, token string, c PublishCommand, definitionRef flow.Ref, invocationID, stageID string, obs Observation) (*Attempt, *Activation, error) {
	current, activation, err := publisherAttempt(r, token, c)
	if err != nil {
		return nil, nil, err
	}
	// Dispatch session is the live publisher generation. A restarted core
	// cannot revive a possibly surviving process merely from its old token.
	if r.terminal() || r.HasUnresolvedEffects || r.Status == "uncertain" || current.Settled != nil || !slices.Contains(r.Active, current.ID) || current.Dispatch == nil || current.Dispatch.Session != obs.Session || !slices.Contains([]string{"dispatching", "running", "stopping"}, current.Status) || r.Steps[current.StepID].Ref != definitionRef || activation.InvocationID != invocationID || activation.StageID != stageID {
		return nil, nil, local.Reject("publisher_frozen", "terminal, inactive or fenced publishers cannot create new publications")
	}
	return current, activation, nil
}

// Store's exact-command path never re-runs the writer. Current read access is
// still required before releasing a retained receipt payload.
func (e *Engine) publisherReceipt(ctx context.Context, token string, c PublishCommand, result local.ApplyResult, err error) (local.ApplyResult, error) {
	if err != nil {
		return result, err
	}
	current, _, readErr := e.load(ctx, c.RunID)
	if readErr == nil {
		_, _, readErr = publisherAttempt(current, token, c)
	}
	if readErr != nil {
		return local.ApplyResult{}, local.Reject("publisher_forbidden", "publisher no longer has receipt read access")
	}
	return result, nil
}

// Counter reset scope is the Attempt, not a changed dimension or temporary
// omission. Compare all retained observations so remove/re-add cannot reset it.
func checkCounterProgress(previous []Publication, mappings []flow.Mapping, c PublishCommand) error {
	value, err := flow.Parse(c.Value, "json")
	if err != nil {
		return err
	}
	current := map[string]float64{}
	for _, mapping := range mappings {
		if mapping.Hook == c.Hook && mapping.Kind == "counter" {
			if n, exists := flow.JSONPointer(value, mapping.Field); exists {
				current[mapping.Field], _ = n.(json.Number).Float64()
			}
		}
	}
	if len(current) == 0 {
		return nil
	}
	for _, old := range previous {
		if old.AttemptID != c.AttemptID || old.Hook != c.Hook || old.Kind != "state" {
			continue
		}
		oldValue, err := flow.Parse(old.Value, "json")
		if err != nil {
			return local.ErrIntegrity
		}
		for field, now := range current {
			if n, exists := flow.JSONPointer(oldValue, field); exists {
				before, ok := n.(json.Number)
				if !ok {
					return local.ErrIntegrity
				}
				n, err := before.Float64()
				if err != nil {
					return local.ErrIntegrity
				}
				if n > now {
					return local.Reject("counter_decreased", "cumulative counter cannot decrease within an Attempt")
				}
			}
		}
	}
	return nil
}

type HookReadView struct {
	Kind           string               `json:"kind"`
	Availability   string               `json:"availability"`
	Freshness      string               `json:"freshness"`
	Count          int64                `json:"count"`
	LatestState    *Publication         `json:"latest_state,omitempty"`
	LatestArtifact *ArtifactPublication `json:"latest_artifact,omitempty"`
	LatestClosure  *ArtifactClosure     `json:"latest_closure,omitempty"`
}

type StepReadView struct {
	SchemaVersion string                  `json:"schema_version"`
	RunID         string                  `json:"run_id"`
	StepID        string                  `json:"step_instance_id"`
	AttemptID     string                  `json:"attempt_id"`
	RunVersion    int64                   `json:"run_version"`
	EventSequence int64                   `json:"event_sequence"`
	Cut           int64                   `json:"cut"`
	AsOf          Observation             `json:"as_of"`
	RunStatus     string                  `json:"run_status"`
	StepStatus    string                  `json:"step_status"`
	AttemptStatus string                  `json:"attempt_status"`
	Restricted    bool                    `json:"restricted"`
	Frozen        bool                    `json:"frozen"`
	Hooks         map[string]HookReadView `json:"hooks"`
}

func (e *Engine) publisherStatus(ctx context.Context, token string, c PublishCommand) (StepReadView, error) {
	r, read, err := e.load(ctx, c.RunID)
	if err != nil {
		return StepReadView{}, local.Reject("publisher_forbidden", "publisher has no current access to this namespace")
	}
	a, activation, err := publisherAttempt(r, token, c)
	if err != nil {
		return StepReadView{}, err
	}
	plan, err := r.planFor(activation.InvocationID)
	if err != nil {
		return StepReadView{}, err
	}
	asOf := e.clock.now()
	version := stepReadVersion
	if isArtifactPublicationState(r.SchemaVersion) {
		version = CoreArtifactPublicationStepReadVersion
	}
	if isArtifactClosureState(r.SchemaVersion) {
		version = CoreArtifactClosureStepReadVersion
	}
	if isPublicationSubscriptionState(r.SchemaVersion) {
		version = CorePublicationSubscriptionStepReadVersion
	}
	if isPublicationChecksState(r.SchemaVersion) {
		version = CorePublicationChecksStepReadVersion
	}
	if isPublicationNewOnlyState(r.SchemaVersion) {
		version = CorePublicationNewOnlyStepReadVersion
	}
	if isPublicationFailureState(r.SchemaVersion) {
		version = CorePublicationFailureStepReadVersion
	}
	if isDecisionState(r.SchemaVersion) {
		version = CoreDecisionStepReadVersion
	} else if isWorkspaceTreeState(r.SchemaVersion) {
		version = CoreWorkspaceTreeStepReadVersion
	} else if isWorkspaceState(r.SchemaVersion) {
		version = CoreWorkspaceStepReadVersion
	} else if isForkState(r.SchemaVersion) {
		version = CoreForkStepReadVersion
	} else if isActionDeliveryState(r.SchemaVersion) {
		version = CoreActionDeliveryStepReadVersion
	} else if isActionGrantAdmissionState(r.SchemaVersion) {
		version = CoreActionGrantAdmissionStepReadVersion
	} else if isActionAdmissionState(r.SchemaVersion) {
		version = CoreActionAdmissionStepReadVersion
	} else if isActionIntentState(r.SchemaVersion) {
		version = CoreActionIntentStepReadVersion
	}
	out := StepReadView{version, r.ID, a.StepID, a.ID, read.Snapshot.Version, read.Snapshot.EventSeq, read.Cut, asOf, r.Status, r.Steps[a.StepID].Status, a.Status, r.restrictedFor(activation.InvocationID) || r.CancelRequested, r.terminal() || r.Status == "uncertain" || a.Settled != nil || !slices.Contains(r.Active, a.ID) || r.HasUnresolvedEffects || a.Dispatch == nil || a.Dispatch.Session != asOf.Session, map[string]HookReadView{}}
	for name, hook := range plan.Steps[activation.StageID].Hooks {
		view := HookReadView{Kind: hook.Kind, Availability: "unpublished", Freshness: "unknown"}
		for _, pub := range r.Publications {
			if pub.AttemptID == a.ID && pub.Hook == name {
				view.Count++
				view.Availability = "published"
				if hook.Kind == "state" && (view.LatestState == nil || view.LatestState.Version < pub.Version) {
					copy := pub
					view.LatestState = &copy
				}
			}
		}
		if hook.Kind == "artifact" {
			for _, publication := range r.ArtifactPublications {
				if publication.StepID == a.StepID && publication.Hook == name {
					view.Count++
					view.Availability = "published"
					copy := publication
					view.LatestArtifact = &copy
				}
			}
			for _, closure := range r.ArtifactClosures {
				if closure.StepID == a.StepID && closure.Hook == name {
					copy := closure
					view.LatestClosure = &copy
				}
			}
		}
		if p := view.LatestState; p != nil && hook.FreshnessMS != nil && p.Received.Session == asOf.Session && asOf.MonotonicMS >= p.Received.MonotonicMS {
			view.Freshness = "stale"
			if asOf.MonotonicMS-p.Received.MonotonicMS < *hook.FreshnessMS {
				view.Freshness = "fresh"
			}
		}
		out.Hooks[name] = view
	}
	return out, nil
}

func (e *Engine) serveSteps() (string, func(), error) {
	// Both qualified platforms provide /tmp. Avoid Darwin's long user-specific
	// TMPDIR exceeding sockaddr_un; the randomly named directory is owner-only.
	dir, err := os.MkdirTemp("/tmp", "prifly-step-")
	if err != nil {
		return "", nil, local.Reject("local_socket_unavailable", "Pri-Fly cannot create the local worker socket; allow local Unix sockets and retry the worker step")
	}
	path := filepath.Join(dir, "api.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, local.Reject("local_socket_unavailable", "Pri-Fly cannot create the local worker socket; allow local Unix sockets and retry the worker step")
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		_ = os.RemoveAll(dir)
		return "", nil, local.Reject("local_socket_unavailable", "Pri-Fly cannot create the local worker socket; allow local Unix sockets and retry the worker step")
	}
	handlers := make(chan struct{}, 4)
	server := &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 4096}
	server.SetKeepAlivesEnabled(false)
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		select {
		case handlers <- struct{}{}:
			defer func() { <-handlers }()
		default:
			stepHTTPError(w, local.Reject("publisher_busy", "publisher request capacity is exhausted"))
			return
		}
		if len(request.Header.Values("Authorization")) != 1 || !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
			stepHTTPError(w, local.Reject("publisher_forbidden", "Bearer authentication is required"))
			return
		}
		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/publish" && request.URL.RawQuery == "":
			request.Body = http.MaxBytesReader(w, request.Body, maxPublishBodyBytes)
			data, err := io.ReadAll(request.Body)
			if err != nil {
				stepHTTPError(w, local.Reject("invalid_publication", "body is unavailable or exceeds the publication bound"))
				return
			}
			command, err := ParsePublishCommand(data)
			if err != nil {
				stepHTTPError(w, local.Reject("invalid_publication", "request must satisfy the closed JSON publication contract"))
				return
			}
			result, err := e.Publish(ctx, token, command)
			if err != nil {
				stepHTTPError(w, err)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(result.Receipt.Result)
		case request.Method == http.MethodGet && request.URL.Path == "/status":
			query, err := url.ParseQuery(request.URL.RawQuery)
			if err != nil || len(query) != 4 {
				stepHTTPError(w, local.Reject("invalid_publication", "status requires exact run, step, attempt and envelope identities"))
				return
			}
			for _, key := range []string{"run_id", "step_instance_id", "attempt_id", "envelope_digest"} {
				if len(query[key]) != 1 || query.Get(key) == "" {
					stepHTTPError(w, local.Reject("invalid_publication", "status identity is missing or ambiguous"))
					return
				}
			}
			view, err := e.publisherStatus(ctx, token, PublishCommand{RunID: query.Get("run_id"), StepID: query.Get("step_instance_id"), AttemptID: query.Get("attempt_id"), EnvelopeDigest: query.Get("envelope_digest")})
			if err != nil {
				stepHTTPError(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(view)
		default:
			stepHTTPError(w, local.Reject("unsupported_request", "only POST /publish and GET /status are supported"))
		}
	})
	go func() { _ = server.Serve(&stepListener{Listener: listener, slots: make(chan struct{}, 16)}) }()
	var once sync.Once
	closeServer := func() { once.Do(func() { _ = server.Close(); _ = os.RemoveAll(dir) }) }
	return path, closeServer, nil
}

func localWorkerSocketAvailable() bool {
	dir, err := os.MkdirTemp("/tmp", "prifly-probe-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	listener, err := net.Listen("unix", filepath.Join(dir, "api.sock"))
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

func stepHTTPError(w http.ResponseWriter, err error) {
	problem, _ := ProblemFor(err)
	status := http.StatusServiceUnavailable
	var rejected *local.Rejection
	if errors.As(err, &rejected) {
		status = http.StatusBadRequest
		switch rejected.Code {
		case "publisher_forbidden":
			status = http.StatusForbidden
		case "state_version_conflict", "event_key_conflict", "artifact_key_conflict", "artifact_digest_mismatch", "artifact_size_mismatch", "publisher_frozen", "publication_restricted":
			status = http.StatusConflict
		case "publication_rate_exhausted", "publication_count_exhausted", "publication_budget_exhausted", "publisher_busy":
			status = http.StatusTooManyRequests
		}
	} else if errors.Is(err, local.ErrCommandConflict) {
		status = http.StatusConflict
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}

// Cap connections before net/http starts goroutines, including slow headers.
type stepListener struct {
	net.Listener
	slots chan struct{}
}

func (l *stepListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &stepConn{Conn: conn, release: func() { <-l.slots }}, nil
		default:
			_ = conn.Close()
		}
	}
}

type stepConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *stepConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}
