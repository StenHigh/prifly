package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	AuthorityPackagesMigrationVersion = "authority-packages/2"
	CoreSchemaMigrationStateVersion   = "core-state/45"
	CoreSchemaMigrationReadVersion    = "core-read/45"
)

// SchemaMigrationSelection names one exact schema and its direct supplier.
type SchemaMigrationSelection struct {
	SchemaRef   flow.Ref `json:"schema_ref"`
	SupplierRef flow.Ref `json:"supplier_ref"`
}

type SchemaMigrationRequest struct {
	SchemaVersion string                     `json:"schema_version"`
	ConsumerRef   flow.Ref                   `json:"consumer_ref"`
	Selections    []SchemaMigrationSelection `json:"selections"`
}

type SchemaMigrationPackageFact struct {
	Ref          flow.Ref `json:"ref"`
	Generation   int64    `json:"generation"`
	Status       string   `json:"status"`
	StatusReason string   `json:"status_reason"`
	TrustReason  string   `json:"trust_reason"`
}

type SchemaMigrationSchema struct {
	SchemaRef   flow.Ref                     `json:"schema_ref"`
	SupplierRef *flow.Ref                    `json:"supplier_ref,omitempty"`
	Candidates  []flow.Ref                   `json:"candidates"`
	Revocations []SchemaMigrationPackageFact `json:"revocations"`
}

type SchemaMigrationRun struct {
	RunID      string     `json:"run_id"`
	Version    int64      `json:"version"`
	Status     string     `json:"status"`
	SchemaRefs []flow.Ref `json:"schema_refs"`
}

// SchemaMigrationPlan is a read-only review of an exact authority cut.
type SchemaMigrationPlan struct {
	SchemaVersion   string                       `json:"schema_version"`
	AuthorityID     string                       `json:"authority_id"`
	ControlEpoch    int64                        `json:"control_epoch"`
	PackagesVersion int64                        `json:"packages_version"`
	ControlVersion  int64                        `json:"control_version"`
	Cut             int64                        `json:"cut"`
	Request         SchemaMigrationRequest       `json:"request"`
	Packages        []SchemaMigrationPackageFact `json:"packages"`
	Schemas         []SchemaMigrationSchema      `json:"schemas"`
	Runs            []SchemaMigrationRun         `json:"runs"`
	Complete        bool                         `json:"complete"`
	Applicable      bool                         `json:"applicable"`
	Reasons         []string                     `json:"reasons"`
	ReviewDigest    string                       `json:"review_digest"`
}

// SchemaMigrationDecision preserves owner consent without restoring a package.
type SchemaMigrationDecision struct {
	SchemaVersion string              `json:"schema_version"`
	ID            string              `json:"id"`
	Actor         string              `json:"actor"`
	Reason        string              `json:"reason"`
	CommandID     string              `json:"command_id"`
	CommitCut     int64               `json:"commit_cut"`
	Plan          SchemaMigrationPlan `json:"plan"`
	Decided       Observation         `json:"decided"`
}

// SchemaMigrationBinding is provenance of the limited admission in a new Run.
type SchemaMigrationBinding struct {
	DecisionID   string                     `json:"decision_id"`
	ReviewDigest string                     `json:"review_digest"`
	ConsumerRef  flow.Ref                   `json:"consumer_ref"`
	Selections   []SchemaMigrationSelection `json:"selections"`
}

type SchemaMigrationResult struct {
	SafeNextCommands []string                `json:"safe_next_commands"`
	SchemaVersion    string                  `json:"schema_version"`
	Decision         SchemaMigrationDecision `json:"decision"`
	Receipt          local.AuthorityReceipt  `json:"receipt"`
	Duplicate        bool                    `json:"duplicate"`
}

type SchemaMigrationInspection struct {
	SchemaVersion string                  `json:"schema_version"`
	Decision      SchemaMigrationDecision `json:"decision"`
	Valid         bool                    `json:"valid"`
	Reasons       []string                `json:"reasons"`
}

type SchemaMigrationApplyRequest struct {
	CommandID            string
	Plan                 SchemaMigrationPlan
	ExpectedReviewDigest string
	Reason               string
}

func packageRecordSupported(record PackageRecord) bool {
	return record.SchemaVersion == AuthorityPackagesVersion || record.SchemaVersion == AuthorityPackagesMigrationVersion
}

func packageLifecycleGeneration(pkg PackageEntry) int64 {
	if pkg.LifecycleGeneration == 0 {
		return 1
	}
	return pkg.LifecycleGeneration
}

func migrationPackageFact(pkg PackageEntry) SchemaMigrationPackageFact {
	status := pkg.Status
	if status == "" {
		status = PackageTrusted
	}
	return SchemaMigrationPackageFact{pkg.Ref, packageLifecycleGeneration(pkg), status, pkg.StatusReason, pkg.Trust.Reason}
}

func migrationPlanDigest(plan SchemaMigrationPlan) (string, error) {
	plan.ReviewDigest = ""
	data, err := canonical(plan)
	if err != nil {
		return "", err
	}
	return rawDigest(data), nil
}

func migrationSortRefs(refs []flow.Ref) {
	slices.SortFunc(refs, func(a, b flow.Ref) int { return strings.Compare(a.String(), b.String()) })
}

func migrationInvalid(message string) error { return fault("schema_migration_invalid", message) }

// PrepareSchemaMigration verifies all scope bytes before returning an applicable plan.
// Reads are fenced by two complete population scans at the same authority cut.
func (e *Engine) PrepareSchemaMigration(ctx context.Context, request SchemaMigrationRequest) (SchemaMigrationPlan, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return SchemaMigrationPlan{}, err
	}
	if request.SchemaVersion != "schema-migration-request/1" || request.ConsumerRef == (flow.Ref{}) {
		return SchemaMigrationPlan{}, migrationInvalid("an exact consumer and request version are required")
	}
	request.Selections = append([]SchemaMigrationSelection{}, request.Selections...)
	requestBytes, err := canonical(request)
	if err != nil {
		return SchemaMigrationPlan{}, err
	}
	if err := validateSchemaMigrationJSON("SchemaMigrationRequestV1", requestBytes); err != nil {
		return SchemaMigrationPlan{}, err
	}

	slices.SortFunc(request.Selections, func(a, b SchemaMigrationSelection) int {
		return strings.Compare(a.SchemaRef.String(), b.SchemaRef.String())
	})
	selected := map[flow.Ref]flow.Ref{}
	for _, s := range request.Selections {
		if s.SchemaRef == (flow.Ref{}) || s.SupplierRef == (flow.Ref{}) {
			return SchemaMigrationPlan{}, migrationInvalid("each selection requires exact schema and supplier refs")
		}
		if _, ok := selected[s.SchemaRef]; ok {
			return SchemaMigrationPlan{}, migrationInvalid("a schema may be selected only once")
		}
		selected[s.SchemaRef] = s.SupplierRef
	}
	// The first scan establishes the cut before any authority or filesystem reads.
	var initialRecords, initialBytes int64
	initial, err := e.Store.ScanSnapshots(ctx, func(snapshot local.Snapshot) error {
		initialRecords++
		initialBytes += int64(len(snapshot.Data))
		if initialRecords > maxPinScanRuns || initialBytes > local.MaxScanBytes {
			return &local.ScanLimitError{Reason: "migration review", Records: initialRecords, RestoredBytes: initialBytes, RecordLimit: maxPinScanRuns, ByteLimit: local.MaxScanBytes}
		}
		return nil
	})
	if err != nil {
		var limit *local.ScanLimitError
		if !errors.As(err, &limit) {
			return SchemaMigrationPlan{}, err
		}
	}
	record, version, err := e.readPackages(ctx)
	if err != nil {
		return SchemaMigrationPlan{}, err
	}
	control, controlVersion, err := e.Control(ctx)
	if err != nil {
		return SchemaMigrationPlan{}, err
	}
	plan := SchemaMigrationPlan{SchemaVersion: "schema-migration-plan/1", AuthorityID: e.Installation.ID, ControlEpoch: control.ControlEpoch, PackagesVersion: version, ControlVersion: controlVersion, Cut: initial.Cut, Request: request, Packages: []SchemaMigrationPackageFact{}, Schemas: []SchemaMigrationSchema{}, Runs: []SchemaMigrationRun{}, Complete: true, Applicable: true, Reasons: []string{}}
	byRef := map[flow.Ref]PackageEntry{}
	for _, pkg := range record.Packages {
		byRef[pkg.Ref] = pkg
	}
	consumer, ok := byRef[request.ConsumerRef]
	if !ok {
		return plan, fault("schema_migration_scope_mismatch", "consumer is not an exact installed package")
	}
	resolvable := resolvablePackages(record)
	closure := map[flow.Ref]bool{}
	var visit func(flow.Ref, int) error
	visit = func(ref flow.Ref, depth int) error {
		if depth > MaxPackageDepth {
			return fault("schema_migration_scope_mismatch", "consumer dependency closure exceeds the depth limit")
		}
		if closure[ref] {
			return nil
		}
		pkg, ok := byRef[ref]
		if !ok {
			return fault("schema_migration_scope_mismatch", "consumer dependency is not installed")
		}
		closure[ref] = true
		for _, dep := range pkg.Dependencies {
			if err := visit(dep, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(consumer.Ref, 1); err != nil {
		return plan, err
	}
	schemaOwners := map[flow.Ref][]flow.Ref{}
	components := map[flow.Ref]string{}
	for ref := range closure {
		pkg := byRef[ref]
		plan.Packages = append(plan.Packages, migrationPackageFact(pkg))
		if !resolvable[ref] {
			plan.Reasons = append(plan.Reasons, "consumer closure package is not resolvable: "+ref.String())
		}
		report, err := e.VerifyPackage(ctx, ref.ID, ref.Version)
		if err != nil || !report.ManifestValid || len(report.Missing) != 0 || len(report.Mismatched) != 0 {
			return plan, fault("schema_migration_integrity", "consumer closure contains missing or changed sealed bytes")
		}
		for _, c := range pkg.Components {
			if previous, ok := components[c.Ref]; ok && previous != c.Kind {
				return plan, fault("schema_migration_integrity", "component ref has inconsistent kinds")
			}
			components[c.Ref] = c.Kind
			if c.Kind == "schema" {
				schemaOwners[c.Ref] = append(schemaOwners[c.Ref], ref)
			}
		}
	}
	for schema, candidates := range schemaOwners {
		migrationSortRefs(candidates)
		item := SchemaMigrationSchema{SchemaRef: schema, Candidates: []flow.Ref{}, Revocations: []SchemaMigrationPackageFact{}}
		for _, candidate := range candidates {
			if resolvable[candidate] {
				item.Candidates = append(item.Candidates, candidate)
			}
		}
		for _, pkg := range record.Packages {
			if pkg.Status == PackageRevoked {
				for _, c := range pkg.Components {
					if c.Ref == schema {
						if c.Kind != "schema" {
							return plan, fault("schema_migration_integrity", "revoked ref is not exclusively a schema")
						}
						item.Revocations = append(item.Revocations, migrationPackageFact(pkg))
						break
					}
				}
			}
		}
		if len(item.Revocations) == 0 {
			continue
		}
		slices.SortFunc(item.Revocations, func(a, b SchemaMigrationPackageFact) int { return strings.Compare(a.Ref.String(), b.Ref.String()) })
		if supplier, ok := selected[schema]; ok {
			if !slices.Contains(item.Candidates, supplier) {
				return plan, fault("schema_migration_scope_mismatch", "selected supplier is not a resolvable direct schema owner in consumer closure")
			}
			item.SupplierRef = &supplier
			delete(selected, schema)
		} else {
			plan.Reasons = append(plan.Reasons, "select a supplier for schema: "+schema.String())
		}
		plan.Schemas = append(plan.Schemas, item)
	}
	if len(selected) != 0 {
		return plan, fault("schema_migration_scope_mismatch", "selection contains a schema outside the blocked consumer scope")
	}
	if len(plan.Schemas) == 0 {
		plan.Reasons = append(plan.Reasons, "consumer closure has no schemas blocked by a revoked supplier")
	}
	slices.SortFunc(plan.Packages, func(a, b SchemaMigrationPackageFact) int { return strings.Compare(a.Ref.String(), b.Ref.String()) })
	slices.SortFunc(plan.Schemas, func(a, b SchemaMigrationSchema) int {
		return strings.Compare(a.SchemaRef.String(), b.SchemaRef.String())
	})
	wanted := map[flow.Ref]bool{}
	for _, s := range plan.Schemas {
		wanted[s.SchemaRef] = true
	}
	var records, restored int64
	scan, err := e.Store.ScanSnapshots(ctx, func(snapshot local.Snapshot) error {
		records++
		restored += int64(len(snapshot.Data))
		if records > maxPinScanRuns || restored > local.MaxScanBytes {
			return &local.ScanLimitError{Reason: "migration review", Records: records, RestoredBytes: restored, RecordLimit: maxPinScanRuns, ByteLimit: local.MaxScanBytes}
		}
		var run Run
		if err := decodeState(snapshot.Data, &run); err != nil {
			return err
		}
		affected := []flow.Ref{}
		for _, ref := range runPackageRefs(run) {
			if wanted[ref] && !slices.Contains(affected, ref) {
				affected = append(affected, ref)
			}
		}
		if len(affected) > 0 {
			migrationSortRefs(affected)
			plan.Runs = append(plan.Runs, SchemaMigrationRun{run.ID, snapshot.Version, run.Status, affected})
		}
		return nil
	})
	if err != nil {
		var limit *local.ScanLimitError
		if !errors.As(err, &limit) {
			return plan, err
		}
		plan.Complete = false
		plan.Reasons = append(plan.Reasons, "Run scan exceeded review limits")
	}
	if scan.Cut != initial.Cut {
		return plan, fault("schema_migration_plan_stale", "authority changed during preparation; prepare again")
	}
	if !scan.Complete {
		plan.Complete = false
	}
	slices.Sort(plan.Reasons)
	plan.Applicable = plan.Complete && len(plan.Reasons) == 0
	plan.ReviewDigest, err = migrationPlanDigest(plan)
	return plan, err
}

// ApplySchemaMigration records the whole reviewed scope in one authority command.
func (e *Engine) ApplySchemaMigration(ctx context.Context, request SchemaMigrationApplyRequest) (SchemaMigrationResult, error) {
	result := SchemaMigrationResult{SchemaVersion: "schema-migration-result/1", SafeNextCommands: []string{}}
	if e.ReadOnly {
		return result, local.ErrReadOnly
	}
	if _, err := e.readAccess(ctx); err != nil {
		return result, err
	}
	if request.CommandID == "" || strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 4096 {
		return result, migrationInvalid("explicit command identity and owner reason are required")
	}
	payload, err := canonical(struct {
		Operation      string              `json:"operation"`
		Plan           SchemaMigrationPlan `json:"plan"`
		ExpectedDigest string              `json:"expected_review_digest"`
		Reason         string              `json:"reason"`
	}{"package.schema-migration.apply", request.Plan, request.ExpectedReviewDigest, request.Reason})
	if err != nil {
		return result, err
	}
	if len(payload) > local.MaxCommandBytes {
		return result, fault("schema_migration_incomplete", "review exceeds the authority command budget")
	}
	command := local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthorityPackagesKey, Payload: payload, ExpectedVersion: &request.Plan.PackagesVersion}
	receipt, err := e.Store.LookupAuthorityReceipt(ctx, command)
	if err == nil {
		result.Receipt = receipt
		result.Duplicate = true
		if receipt.Rejection == nil {
			if err := decode(receipt.Result, &result.Decision); err != nil {
				return result, err
			}
		}
		if receipt.Rejection == nil {
			result.SafeNextCommands = migrationNextCommands(result.Decision.ID)
		}
		return result, nil
	}
	if !errors.Is(err, local.ErrNotFound) {
		return result, err
	}
	plan := request.Plan
	planBytes, err := canonical(plan)
	if err != nil {
		return result, err
	}
	if err := validateSchemaMigrationJSON("SchemaMigrationPlanV1", planBytes); err != nil {
		return result, err
	}
	digest, err := migrationPlanDigest(plan)
	if err != nil {
		return result, err
	}
	if plan.SchemaVersion != "schema-migration-plan/1" || digest != plan.ReviewDigest || request.ExpectedReviewDigest != digest {
		return result, migrationInvalid("review digest differs from the exact plan")
	}
	if !plan.Applicable || !plan.Complete {
		return result, fault("schema_migration_incomplete", "plan is incomplete or not applicable; prepare again")
	}
	if plan.AuthorityID != e.Installation.ID {
		return result, fault("schema_migration_scope_mismatch", "plan belongs to another authority")
	}
	current, err := e.PrepareSchemaMigration(ctx, plan.Request)
	stale := false
	if err != nil {
		var refusal *Fault
		if !errors.As(err, &refusal) || refusal.Code != "schema_migration_scope_mismatch" {
			return result, err
		}
		stale = true
	} else {
		stale = current.ReviewDigest != plan.ReviewDigest
	}
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return result, err
	}
	decision := SchemaMigrationDecision{SchemaVersion: "schema-migration-decision/1", ID: derivedID("schema-migration", e.Installation.ID, e.owner, request.CommandID), Actor: e.owner, Reason: request.Reason, CommandID: request.CommandID, Plan: plan, Decided: e.clock.now(), CommitCut: plan.Cut + 1}
	if record.SchemaVersion == AuthorityPackagesVersion {
		record.SchemaVersion = AuthorityPackagesMigrationVersion
		for i := range record.Packages {
			record.Packages[i].LifecycleGeneration = packageLifecycleGeneration(record.Packages[i])
		}
	}
	record.SchemaMigrations = append(record.SchemaMigrations, decision)
	preparedJSON, err := json.Marshal(record)
	if err != nil {
		return result, err
	}
	if len(preparedJSON) > local.MaxSnapshotBytes {
		return result, fault("schema_migration_incomplete", "decision exceeds the authority state budget")
	}
	prepared, err := canonicalState(record)
	if err != nil {
		return result, err
	}
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		return result, err
	}
	if len(decisionJSON) > local.MaxCommandBytes {
		return result, fault("schema_migration_incomplete", "decision exceeds the command result budget")
	}
	decisionBytes, err := canonical(decision)
	if err != nil {
		return result, err
	}
	command.ExpectedVersion = &plan.PackagesVersion
	command.ExpectedCut = &plan.Cut
	command.StaleRejectionCode = "schema_migration_plan_stale"
	for _, run := range plan.Runs {
		command.RunPins = append(command.RunPins, local.RunPin{ID: run.RunID, Version: run.Version})
	}
	command.AuthorityGuards = []local.AuthorityGuard{{Key: AuthorityControlKey, Check: func(snapshot local.AuthoritySnapshot) error {
		control, err := decodeControl(snapshot.Data)
		if err != nil {
			return err
		}
		if !control.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) {
			return local.Reject("object_access_denied", "the session principal cannot approve schema migration")
		}
		if snapshot.Version != plan.ControlVersion || control.ControlEpoch != plan.ControlEpoch {
			return local.Reject("schema_migration_plan_stale", "control changed; prepare again")
		}
		return e.controlCompatible(control)
	}}}
	applied, err := e.Store.ApplyAuthority(ctx, command, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		if stale {
			return local.AuthorityChange{}, local.Reject("schema_migration_plan_stale", "authority changed after review; prepare again")
		}
		return local.AuthorityChange{Data: prepared, Result: decisionBytes}, nil
	})
	result.Receipt = applied.Receipt
	result.Duplicate = applied.Duplicate
	if err == nil && applied.Receipt.Rejection == nil {
		if err := decode(applied.Receipt.Result, &result.Decision); err != nil {
			return result, err
		}
	}
	if err == nil && result.Receipt.Rejection == nil {
		result.SafeNextCommands = migrationNextCommands(result.Decision.ID)
	}
	return result, err
}

func migrationDecision(record PackageRecord, id string) (SchemaMigrationDecision, bool) {
	for _, decision := range record.SchemaMigrations {
		if decision.ID == id {
			return decision, true
		}
	}
	return SchemaMigrationDecision{}, false
}

// migrationDecisionReasons checks current facts, not the plan's old global cut.
func migrationDecisionReasons(record PackageRecord, decision SchemaMigrationDecision) []string {
	reasons := []string{}
	byRef := map[flow.Ref]PackageEntry{}
	for _, pkg := range record.Packages {
		byRef[pkg.Ref] = pkg
	}
	resolvable := resolvablePackages(record)
	for _, fact := range decision.Plan.Packages {
		pkg, ok := byRef[fact.Ref]
		if !ok || migrationPackageFact(pkg) != fact || !resolvable[fact.Ref] {
			reasons = append(reasons, "consumer closure lifecycle changed: "+fact.Ref.String())
		}
	}
	for _, schema := range decision.Plan.Schemas {
		if schema.SupplierRef == nil || !resolvable[*schema.SupplierRef] {
			reasons = append(reasons, "selected schema supplier is not resolvable: "+schema.SchemaRef.String())
		}
		current := []SchemaMigrationPackageFact{}
		for _, pkg := range record.Packages {
			if pkg.Status == PackageRevoked {
				for _, component := range pkg.Components {
					if component.Ref == schema.SchemaRef {
						current = append(current, migrationPackageFact(pkg))
						break
					}
				}
			}
		}
		slices.SortFunc(current, func(a, b SchemaMigrationPackageFact) int { return strings.Compare(a.Ref.String(), b.Ref.String()) })
		if !slices.Equal(current, schema.Revocations) {
			reasons = append(reasons, "revoked suppliers changed: "+schema.SchemaRef.String())
		}
	}
	slices.Sort(reasons)
	return reasons
}

func (e *Engine) InspectSchemaMigration(ctx context.Context, id string) (SchemaMigrationInspection, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return SchemaMigrationInspection{}, err
	}
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return SchemaMigrationInspection{}, err
	}
	decision, ok := migrationDecision(record, id)
	if !ok {
		return SchemaMigrationInspection{}, fault("schema_migration_decision_invalid", "decision is not recorded in this authority")
	}
	reasons := migrationDecisionReasons(record, decision)
	for _, fact := range decision.Plan.Packages {
		report, err := e.VerifyPackage(ctx, fact.Ref.ID, fact.Ref.Version)
		if err != nil || !report.ManifestValid || len(report.Missing) > 0 || len(report.Mismatched) > 0 {
			reasons = append(reasons, "sealed package bytes changed: "+fact.Ref.String())
		}
	}
	slices.Sort(reasons)
	return SchemaMigrationInspection{SchemaVersion: "schema-migration-inspection/1", Decision: decision, Valid: len(reasons) == 0, Reasons: reasons}, nil
}

func (e *Engine) schemaMigrationBinding(ctx context.Context, id string, workflow flow.Ref, defs []PinnedDefinition) (*SchemaMigrationBinding, error) {
	if id == "" {
		return nil, nil
	}
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return nil, err
	}
	decision, ok := migrationDecision(record, id)
	if !ok {
		return nil, fault("schema_migration_decision_invalid", "decision is not recorded in this authority")
	}
	owned := false
	for _, pkg := range record.Packages {
		if pkg.Ref == decision.Plan.Request.ConsumerRef {
			for _, component := range pkg.Components {
				if component.Kind == "workflow" && component.Ref == workflow {
					owned = true
				}
			}
		}
	}
	if !owned {
		return nil, fault("schema_migration_scope_mismatch", "selected workflow does not belong to the exact reviewed consumer package")
	}
	binding := &SchemaMigrationBinding{DecisionID: id, ReviewDigest: decision.Plan.ReviewDigest, ConsumerRef: decision.Plan.Request.ConsumerRef, Selections: []SchemaMigrationSelection{}}
	for _, schema := range decision.Plan.Schemas {
		for _, def := range defs {
			if def.Kind == "schema" && def.Ref == schema.SchemaRef && schema.SupplierRef != nil {
				binding.Selections = append(binding.Selections, SchemaMigrationSelection{schema.SchemaRef, *schema.SupplierRef})
				break
			}
		}
	}
	if err := e.validateSchemaMigrationBinding(ctx, record, binding); err != nil {
		return nil, err
	}
	return binding, nil
}

func (e *Engine) validateSchemaMigrationBinding(ctx context.Context, record PackageRecord, binding *SchemaMigrationBinding) error {
	if binding == nil {
		return nil
	}
	decision, ok := migrationDecision(record, binding.DecisionID)
	if !ok || decision.Plan.ReviewDigest != binding.ReviewDigest || decision.Plan.Request.ConsumerRef != binding.ConsumerRef {
		return fault("schema_migration_decision_invalid", "binding differs from its recorded decision")
	}
	if reasons := migrationDecisionReasons(record, decision); len(reasons) > 0 {
		return fault("schema_migration_decision_invalid", reasons[0])
	}
	approved := map[flow.Ref]flow.Ref{}
	for _, schema := range decision.Plan.Schemas {
		if schema.SupplierRef != nil {
			approved[schema.SchemaRef] = *schema.SupplierRef
		}
	}
	seen := map[flow.Ref]bool{}
	for _, selection := range binding.Selections {
		if seen[selection.SchemaRef] || approved[selection.SchemaRef] != selection.SupplierRef {
			return fault("schema_migration_scope_mismatch", "binding contains a schema or supplier outside its decision")
		}
		seen[selection.SchemaRef] = true
	}
	for _, fact := range decision.Plan.Packages {
		report, err := e.VerifyPackage(ctx, fact.Ref.ID, fact.Ref.Version)
		if err != nil || !report.ManifestValid || len(report.Missing) > 0 || len(report.Mismatched) > 0 {
			return fault("schema_migration_integrity", "reviewed package bytes changed")
		}
	}
	return nil
}

func schemaMigrationBindingInvariant(r Run) error {
	if r.SchemaVersion != CoreSchemaMigrationStateVersion {
		if r.SchemaMigration != nil {
			return local.ErrIntegrity
		}
		return nil
	}
	binding := r.SchemaMigration
	if binding == nil || binding.DecisionID == "" || binding.ReviewDigest == "" || binding.ConsumerRef == (flow.Ref{}) || binding.Selections == nil {
		return local.ErrIntegrity
	}
	seen := map[flow.Ref]bool{}
	for _, s := range binding.Selections {
		if s.SchemaRef == (flow.Ref{}) || s.SupplierRef == (flow.Ref{}) || seen[s.SchemaRef] {
			return local.ErrIntegrity
		}
		seen[s.SchemaRef] = true
	}
	return nil
}

func validateSchemaMigrationJSON(name string, data []byte) error {
	schema, err := PublicSchema(name)
	if err != nil {
		return err
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		return err
	}
	ref := flow.Ref{ID: "core:schema/schema-migration", Version: "1.0.0", Digest: digest}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		return migrationInvalid("invalid closed " + name + " contract: " + err.Error())
	}
	return nil
}

func ParseSchemaMigrationRequest(data []byte) (SchemaMigrationRequest, error) {
	var request SchemaMigrationRequest
	if err := validateSchemaMigrationJSON("SchemaMigrationRequestV1", data); err != nil {
		return request, err
	}
	err := decode(data, &request)
	return request, err
}

func ParseSchemaMigrationPlan(data []byte) (SchemaMigrationPlan, error) {
	var plan SchemaMigrationPlan
	if err := validateSchemaMigrationJSON("SchemaMigrationPlanV1", data); err != nil {
		return plan, err
	}
	err := decode(data, &plan)
	return plan, err
}

func schemaMigrationDefinitionsInvariant(r Run) error {
	if r.SchemaMigration == nil {
		return nil
	}
	for _, selection := range r.SchemaMigration.Selections {
		found := false
		for _, def := range r.Definitions {
			if def.Ref == selection.SchemaRef && def.Kind == "schema" {
				found = true
				break
			}
		}
		if !found {
			return local.ErrIntegrity
		}
	}
	return nil
}

func migrationNextCommands(id string) []string {
	return []string{"package schema-migration inspect --id " + id, "run start --workflow FILE --brief FILE --schema-migration " + id}
}
