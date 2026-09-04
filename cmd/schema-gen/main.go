// schema-gen is a development tool, never part of the execution path. It emits
// closed JSON DTO shapes plus explicit profile discriminators. Semantic authority,
// graph and payload-schema checks remain in their production handlers.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type generator struct {
	defs                  map[string]any
	core                  bool
	invocations           bool
	repeats               bool
	contexts              bool
	sessions              bool
	waivers               bool
	parallel              bool
	maps                  bool
	waits                 bool
	guards                bool
	reportedCosts         bool
	artifacts             bool
	closures              bool
	subscriptions         bool
	publicationChecks     bool
	publicationNewOnly    bool
	publicationFailure    bool
	actionIntents         bool
	actionAdmissions      bool
	actionGrantAdmissions bool
	actionDeliveries      bool
	forks                 bool
	workspaces            bool
	workspaceTrees        bool
	decisionState         bool
}

func (g *generator) schema(t reflect.Type) map[string]any {
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	if t == reflect.TypeFor[json.Number]() {
		return map[string]any{"type": "number"}
	}
	if t.Kind() == reflect.Pointer {
		return nullable(g.schema(t.Elem()))
	}
	switch t.Kind() {
	case reflect.Struct:
		name := path.Base(t.PkgPath()) + "_" + t.Name()
		if _, exists := g.defs[name]; !exists {
			g.defs[name] = map[string]any{}
			properties := map[string]any{}
			required := []string{}
			var fields func(reflect.Type)
			fields = func(t reflect.Type) {
				for i := 0; i < t.NumField(); i++ {
					field := t.Field(i)
					if field.PkgPath != "" {
						continue
					}
					// Skip before recursion: removing the property afterwards would
					// still add new unreachable definitions to the immutable F1 bundle.
					if !g.core && field.Name == "EffectiveConfiguration" && (t == reflect.TypeFor[prifly.Run]() || t == reflect.TypeFor[prifly.Preview]()) {
						continue
					}
					if !g.core && t == reflect.TypeFor[prifly.Diagnostic]() && field.Name == "ActivationID" {
						continue
					}
					if !g.invocations && invocationField(t, field.Name) || g.invocations && t == reflect.TypeFor[prifly.Run]() && field.Name == "Ready" {
						continue
					}
					if !g.repeats && repeatField(t, field.Name) {
						continue
					}
					if !g.contexts && contextField(t, field.Name) {
						continue
					}
					if !g.sessions && sessionField(t, field.Name) {
						continue
					}
					if !g.waivers && waiverField(t, field.Name) {
						continue
					}
					if !g.parallel && !g.maps && parallelField(t, field.Name) {
						continue
					}
					if !g.maps && !g.waits && mapField(t, field.Name) {
						continue
					}
					if !g.waits && waitField(t, field.Name) {
						continue
					}
					if !g.guards && guardField(t, field.Name) {
						continue
					}
					if !g.reportedCosts && reportedCostField(t, field.Name) {
						continue
					}
					if !g.artifacts && artifactPublicationField(t, field.Name) {
						continue
					}
					if !g.closures && artifactClosureField(t, field.Name) {
						continue
					}
					if !g.subscriptions && publicationSubscriptionField(t, field.Name) {
						continue
					}
					if !g.publicationChecks && publicationChecksField(t, field.Name) {
						continue
					}
					if !g.publicationNewOnly && !g.publicationFailure && publicationNewOnlyField(t, field.Name) {
						continue
					}
					if !g.actionIntents && actionIntentField(t, field.Name) {
						continue
					}
					if !g.actionAdmissions && actionAdmissionField(t, field.Name) {
						continue
					}
					if !g.actionDeliveries && actionDeliveryField(t, field.Name) {
						continue
					}
					if !g.forks && forkField(t, field.Name) {
						continue
					}
					if !g.workspaces && workspaceField(t, field.Name) {
						continue
					}
					if !g.workspaceTrees && workspaceTreeField(t, field.Name) {
						continue
					}
					if !g.decisionState && decisionStateField(t, field.Name) {
						continue
					}
					tag := strings.Split(field.Tag.Get("json"), ",")
					if tag[0] == "-" {
						continue
					}
					if field.Anonymous && tag[0] == "" {
						fields(field.Type)
						continue
					}
					key := tag[0]
					if key == "" {
						key = field.Name
					}
					properties[key] = g.schema(field.Type)
					optional := false
					for _, value := range tag[1:] {
						if value == "omitempty" {
							optional = true
						}
					}
					if !optional {
						required = append(required, key)
					}
				}
			}
			fields(t)
			sort.Strings(required)
			g.defs[name] = map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "minimum": -9007199254740991, "maximum": 9007199254740991}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Map:
		return nullable(map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())})
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nullable(map[string]any{"type": "string", "contentEncoding": "base64"})
		}
		return nullable(map[string]any{"type": "array", "items": g.schema(t.Elem())})
	case reflect.Array:
		return map[string]any{"type": "array", "items": g.schema(t.Elem()), "minItems": t.Len(), "maxItems": t.Len()}
	case reflect.Interface:
		return map[string]any{}
	default:
		panic("unsupported JSON DTO type: " + t.String())
	}
}
func nullable(value map[string]any) map[string]any {
	return map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}
}
func (g *generator) property(def, field string, value any) {
	g.defs[def].(map[string]any)["properties"].(map[string]any)[field] = value
}

// describe annotates one generated field. A shape says what a value may look
// like, never what it means, and a caller who cannot read the meaning here
// reconstructs it by guessing. The annotation changes no validation.
func (g *generator) describe(def, field, text string) {
	definition, ok := g.defs[def].(map[string]any)
	if !ok {
		return
	}
	properties, ok := definition["properties"].(map[string]any)
	if !ok {
		return
	}
	if value, ok := properties[field].(map[string]any); ok {
		value["description"] = text
	}
}
func enum(values ...string) map[string]any { return map[string]any{"enum": values} }

// profileContracts are the generated contract sets, in the order they were
// introduced. Selecting one generates it and everything before it, which is what
// the chains of ORs used to spell out one profile at a time.
var profileContracts = []struct {
	flag, description string
	enable            func(*generator)
}{
	{"core", "generate core-workflow/1 public contracts", func(g *generator) { g.core = true }},
	{"invocations", "generate core invocation state/read version 2 contracts", func(g *generator) { g.invocations = true }},
	{"repeats", "generate core repeat state/read version 3 contracts", func(g *generator) { g.repeats = true }},
	{"contexts", "generate core context state/read version 4 contracts", func(g *generator) { g.contexts = true }},
	{"sessions", "generate assisted session state/read version 5 contracts", func(g *generator) { g.sessions = true }},
	{"waivers", "generate quality waiver state/read version 6 contracts", func(g *generator) { g.waivers = true }},
	{"parallel", "generate branch fan-out state/read version 7 contracts", func(g *generator) { g.parallel = true }},
	{"map", "generate sealed collection state/read version 8 contracts", func(g *generator) { g.maps = true }},
	{"wait", "generate wait registration state/read version 9 contracts", func(g *generator) { g.waits = true }},
	{"guard", "generate live guard state/read version 10 contracts", func(g *generator) { g.guards = true }},
	{"reported-cost", "generate named reported cost state/read version 11 contracts", func(g *generator) { g.reportedCosts = true }},
	{"artifact-publication", "generate early artifact publication state/read version 12 contracts", func(g *generator) { g.artifacts = true }},
	{"artifact-closure", "generate artifact closure state/read version 13 contracts", func(g *generator) { g.closures = true }},
	{"publication-subscription", "generate publication subscription state/read version 14 contracts", func(g *generator) { g.subscriptions = true }},
	{"publication-checks", "generate checked artifact publication state/read version 15 contracts", func(g *generator) { g.publicationChecks = true }},
	{"publication-new-only", "generate new-only publication source state/read version 16 contracts", func(g *generator) { g.publicationNewOnly = true }},
	{"publication-failure", "generate terminal producer-failure interruption state/read version 17 contracts", func(g *generator) { g.publicationFailure = true }},
	{"action-intent", "generate durable ActionIntent proposal state/read version 18 contracts", func(g *generator) { g.actionIntents = true }},
	{"action-admission", "generate durable ActionAdmission state/read version 19 contracts", func(g *generator) { g.actionAdmissions = true }},
	{"action-grant-admission", "generate ActionAdmission exact Grant state/read version 20 contracts", func(g *generator) { g.actionGrantAdmissions = true }},
	{"action-delivery", "generate prepared ActionDelivery state/read version 21 contracts", func(g *generator) { g.actionDeliveries = true }},
	{"fork", "generate linked Run fork state/read version 22 contracts", func(g *generator) { g.forks = true }},
	{"workspace", "generate declared repository workspace state/read version 23 contracts", func(g *generator) { g.workspaces = true }},
	{"workspace-tree", "generate workspace-tree state/read version 24 contracts", func(g *generator) { g.workspaceTrees = true }},
	{"decision-state", "generate decision catalog state/read version 25 contracts", func(g *generator) { g.decisionState = true }},
}

// documentContracts are the author-facing documents, each produced whole by the
// contract package rather than reflected from a Go type.
var documentContracts = []struct {
	flag, description string
	build             func() ([]byte, error)
}{
	{"step-definition-v3", "generate StepDefinition v3 author contract", func() ([]byte, error) { return flow.ProtocolSchema("StepDefinitionV3") }},
	{"step-definition-v4", "generate StepDefinition v4 author contract", func() ([]byte, error) { return flow.ProtocolSchema("StepDefinitionV4") }},
	{"step-definition-v5", "generate StepDefinition v5 author contract", func() ([]byte, error) { return flow.ProtocolSchema("StepDefinitionV5") }},
	{"workflow-revision-v3", "generate WorkflowRevision v3 author contract", func() ([]byte, error) { return flow.ProtocolSchema("WorkflowRevisionV3") }},
	{"publication-source", "generate once artifact publication source author contract", func() ([]byte, error) { return flow.PublicationSourceSchema() }},
	{"publication-source-v2", "generate each-publication source author contract", func() ([]byte, error) { return flow.PublicationSourceSchemaV2() }},
	{"publication-source-v3", "generate once new-only source author contract", func() ([]byte, error) { return flow.PublicationSourceSchemaV3() }},
	{"publication-source-v4", "generate each-publication new-only source author contract", func() ([]byte, error) { return flow.PublicationSourceSchemaV4() }},
	{"publication-source-v5", "generate once source with terminal-failure interruption", func() ([]byte, error) { return flow.PublicationSourceSchemaV5() }},
	{"publication-source-v6", "generate each-publication source with terminal-failure interruption", func() ([]byte, error) { return flow.PublicationSourceSchemaV6() }},
	{"publication-source-v7", "generate once JSON/blob publication source author contract", func() ([]byte, error) { return flow.PublicationSourceSchemaV7() }},
	{"publication-source-v8", "generate each-publication JSON/blob source author contract", func() ([]byte, error) { return flow.PublicationSourceSchemaV8() }},
}

func main() {
	choice := flag.Bool("choice", false, "generate choice-decision/1 public contract")
	decisions := flag.Bool("run-decisions", false, "generate Run decision bridge contracts")
	profile := make([]*bool, len(profileContracts))
	for i, contract := range profileContracts {
		profile[i] = flag.Bool(contract.flag, false, contract.description)
	}
	document := make([]*bool, len(documentContracts))
	for i, contract := range documentContracts {
		document[i] = flag.Bool(contract.flag, false, contract.description)
	}
	flag.Parse()
	selected, exclusive := -1, 0
	for _, chosen := range []*bool{choice, decisions} {
		if *chosen {
			exclusive++
		}
	}
	for i, chosen := range profile {
		if *chosen {
			selected, exclusive = i, exclusive+1
		}
	}
	for i, chosen := range document {
		if *chosen {
			exclusive++
			if exclusive == 1 && flag.NArg() == 0 {
				data, err := documentContracts[i].build()
				if err != nil {
					panic(err)
				}
				_, _ = os.Stdout.Write(append(data, '\n'))
				return
			}
		}
	}
	if flag.NArg() != 0 || exclusive > 1 {
		fmt.Fprintln(os.Stderr, "schema-gen accepts one contract-selection flag")
		os.Exit(2)
	}
	if *choice {
		emit(choiceSchema())
		return
	}
	if *decisions {
		emit(decisionSchema())
		return
	}
	g := generator{defs: map[string]any{}}
	for i := 0; i <= selected; i++ {
		profileContracts[i].enable(&g)
	}
	contracts := map[string]reflect.Type{
		"FoundationRunView":             reflect.TypeFor[prifly.RunView](),
		"FoundationNextView":            reflect.TypeFor[prifly.NextView](),
		"FoundationPreview":             reflect.TypeFor[prifly.Preview](),
		"FoundationStepReadView":        reflect.TypeFor[prifly.StepReadView](),
		"PublishStepPublicationCommand": reflect.TypeFor[prifly.PublishCommand](),
		"StepPublication":               reflect.TypeFor[prifly.Publication](),
		"TelemetryQuery":                reflect.TypeFor[prifly.TelemetryQuery](),
		"TelemetryReport":               reflect.TypeFor[prifly.TelemetryResponse](),
		"TelemetryMetricDescriptor":     reflect.TypeFor[prifly.TelemetryDescriptor](),
		"TelemetrySample":               reflect.TypeFor[prifly.TelemetrySampleData](),
		"TimingTree":                    reflect.TypeFor[prifly.TimingTree](),
		"Duration":                      reflect.TypeFor[prifly.Duration](),
		"CommandReceipt":                reflect.TypeFor[local.Receipt](),
		"LocalContextManifest":          reflect.TypeFor[prifly.ContextManifest](),
		"ReleaseRequest":                reflect.TypeFor[prifly.ReleaseRequest](),
	}
	if g.core {
		contracts = map[string]reflect.Type{
			"CoreRunView":            reflect.TypeFor[prifly.RunView](),
			"CoreRunState":           reflect.TypeFor[prifly.Run](),
			"CorePreview":            reflect.TypeFor[prifly.Preview](),
			"CapabilityManifest":     reflect.TypeFor[prifly.CapabilityManifest](),
			"EffectiveConfiguration": reflect.TypeFor[prifly.EffectiveConfiguration](),
			"CoreConfiguration":      reflect.TypeFor[prifly.Configuration](),
			"JSONProjection":         reflect.TypeFor[prifly.ProjectionManifest](),
		}
	}
	if g.invocations {
		contracts = map[string]reflect.Type{
			"CoreRunViewV2":          reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV2":         reflect.TypeFor[prifly.Run](),
			"CoreNextViewV2":         reflect.TypeFor[prifly.NextView](),
			"CorePreviewV2":          reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocation": reflect.TypeFor[prifly.Invocation](),
			"CoreCapabilitiesV2":     reflect.TypeFor[prifly.CapabilityManifest](),
			"LocalRegistryV2":        reflect.TypeFor[prifly.RegistryFile](),
		}
	}
	if g.repeats {
		contracts = map[string]reflect.Type{
			"CoreRunViewV3":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV3":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV3":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV3":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV3": reflect.TypeFor[prifly.Invocation](),
			"RepeatProgress":           reflect.TypeFor[prifly.RepeatProgress](),
			"RepeatDecision":           reflect.TypeFor[prifly.RepeatDecision](),
		}
	}
	if g.guards {
		contracts = map[string]reflect.Type{
			"CoreRunViewV10":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV10":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV10":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV10":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV10": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":           reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":       reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":    reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":            reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":           reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":            reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":              reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":               reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":         reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":            reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":            reflect.TypeFor[prifly.ContextRequest](),
			"SessionHandoff":            reflect.TypeFor[prifly.SessionHandoff](),
			"SessionTask":               reflect.TypeFor[prifly.SessionTask](),
			"SessionSubmission":         reflect.TypeFor[prifly.SessionSubmission](),
			"Waiver":                    reflect.TypeFor[prifly.Waiver](),
			"ParallelProgress":          reflect.TypeFor[prifly.ParallelProgress](),
			"JoinDecision":              reflect.TypeFor[prifly.JoinDecision](),
			"SealedItem":                reflect.TypeFor[prifly.SealedItem](),
			"WaitRegistration":          reflect.TypeFor[prifly.WaitRegistration](),
			"EventEnvelope":             reflect.TypeFor[prifly.EventEnvelope](),
			"InboxEvent":                reflect.TypeFor[prifly.InboxEvent](),
			"WaitProgress":              reflect.TypeFor[prifly.WaitProgress](),
			"GuardRegistration":         reflect.TypeFor[prifly.GuardRegistration](),
			"GuardObservation":          reflect.TypeFor[prifly.GuardObservation](),
		}
		if g.reportedCosts {
			for old, current := range map[string]string{
				"CoreRunViewV10": "CoreRunViewV11", "CoreRunStateV10": "CoreRunStateV11",
				"CoreNextViewV10": "CoreNextViewV11", "CoreWorkflowInvocationV10": "CoreWorkflowInvocationV11",
				"SessionHandoff": "SessionHandoffV2", "SessionTask": "SessionTaskV2", "SessionSubmission": "SessionSubmissionV2",
			} {
				contracts[current] = contracts[old]
				delete(contracts, old)
			}
			contracts["ReportedCost"] = reflect.TypeFor[prifly.ReportedCost]()
			if g.artifacts {
				for old, current := range map[string]string{
					"CoreRunViewV11": "CoreRunViewV12", "CoreRunStateV11": "CoreRunStateV12",
					"CoreNextViewV11": "CoreNextViewV12", "CoreWorkflowInvocationV11": "CoreWorkflowInvocationV12",
				} {
					contracts[current] = contracts[old]
					delete(contracts, old)
				}
				delete(contracts, "CorePreviewV10")
				contracts["CorePreviewV12"] = reflect.TypeFor[prifly.Preview]()
				contracts["CoreStepReadViewV12"] = reflect.TypeFor[prifly.StepReadView]()
				contracts["PublishStepPublicationCommandV2"] = reflect.TypeFor[prifly.PublishCommand]()
				contracts["ArtifactPublication"] = reflect.TypeFor[prifly.ArtifactPublication]()
				contracts["CoreCapabilitiesV12"] = reflect.TypeFor[prifly.CapabilityManifest]()
				if g.closures {
					for old, current := range map[string]string{
						"CoreRunViewV12": "CoreRunViewV13", "CoreRunStateV12": "CoreRunStateV13",
						"CoreNextViewV12": "CoreNextViewV13", "CoreWorkflowInvocationV12": "CoreWorkflowInvocationV13",
						"CorePreviewV12": "CorePreviewV13", "CoreStepReadViewV12": "CoreStepReadViewV13",
						"PublishStepPublicationCommandV2": "PublishStepPublicationCommandV3", "CoreCapabilitiesV12": "CoreCapabilitiesV13",
					} {
						contracts[current] = contracts[old]
						delete(contracts, old)
					}
					contracts["ArtifactManifest"] = reflect.TypeFor[prifly.ArtifactManifest]()
					contracts["ArtifactClosure"] = reflect.TypeFor[prifly.ArtifactClosure]()
					if g.subscriptions {
						for old, current := range map[string]string{
							"CoreRunViewV13": "CoreRunViewV14", "CoreRunStateV13": "CoreRunStateV14",
							"CoreNextViewV13": "CoreNextViewV14", "CoreWorkflowInvocationV13": "CoreWorkflowInvocationV14",
							"CorePreviewV13": "CorePreviewV14", "CoreStepReadViewV13": "CoreStepReadViewV14",
							"CoreCapabilitiesV13": "CoreCapabilitiesV14",
						} {
							contracts[current] = contracts[old]
							delete(contracts, old)
						}
						contracts["PublicationSubscription"] = reflect.TypeFor[prifly.PublicationSubscription]()
						contracts["PublicationAssignment"] = reflect.TypeFor[prifly.PublicationAssignment]()
						contracts["PublicationSubscriptionHandle"] = reflect.TypeFor[prifly.PublicationSubscriptionHandle]()
						contracts["PublicationCursor"] = reflect.TypeFor[prifly.PublicationCursor]()
						contracts["PublicationDelivery"] = reflect.TypeFor[prifly.PublicationDelivery]()
						if g.publicationChecks {
							for old, current := range map[string]string{
								"CoreRunViewV14": "CoreRunViewV15", "CoreRunStateV14": "CoreRunStateV15",
								"CoreNextViewV14": "CoreNextViewV15", "CoreWorkflowInvocationV14": "CoreWorkflowInvocationV15",
								"CorePreviewV14": "CorePreviewV15", "CoreStepReadViewV14": "CoreStepReadViewV15",
								"CoreCapabilitiesV14": "CoreCapabilitiesV15",
							} {
								contracts[current] = contracts[old]
								delete(contracts, old)
							}
							contracts["PendingArtifactPublication"] = reflect.TypeFor[prifly.PendingArtifactPublication]()
							if g.publicationNewOnly {
								for old, current := range map[string]string{
									"CoreRunViewV15": "CoreRunViewV16", "CoreRunStateV15": "CoreRunStateV16",
									"CoreNextViewV15": "CoreNextViewV16", "CoreWorkflowInvocationV15": "CoreWorkflowInvocationV16",
									"CorePreviewV15": "CorePreviewV16", "CoreStepReadViewV15": "CoreStepReadViewV16",
									"CoreCapabilitiesV15": "CoreCapabilitiesV16",
								} {
									contracts[current] = contracts[old]
									delete(contracts, old)
								}
								if g.publicationFailure {
									for old, current := range map[string]string{
										"CoreRunViewV16": "CoreRunViewV17", "CoreRunStateV16": "CoreRunStateV17",
										"CoreNextViewV16": "CoreNextViewV17", "CoreWorkflowInvocationV16": "CoreWorkflowInvocationV17",
										"CorePreviewV16": "CorePreviewV17", "CoreStepReadViewV16": "CoreStepReadViewV17",
										"CoreCapabilitiesV16": "CoreCapabilitiesV17",
									} {
										contracts[current] = contracts[old]
										delete(contracts, old)
									}
									if g.actionIntents {
										for old, current := range map[string]string{
											"CoreRunViewV17": "CoreRunViewV18", "CoreRunStateV17": "CoreRunStateV18",
											"CoreNextViewV17": "CoreNextViewV18", "CoreWorkflowInvocationV17": "CoreWorkflowInvocationV18",
											"CorePreviewV17": "CorePreviewV18", "CoreStepReadViewV17": "CoreStepReadViewV18",
											"CoreCapabilitiesV17": "CoreCapabilitiesV18",
										} {
											contracts[current] = contracts[old]
											delete(contracts, old)
										}
										contracts["ActionIntent"] = reflect.TypeFor[prifly.ActionIntent]()
										contracts["ActionIntentRecord"] = reflect.TypeFor[prifly.ActionIntentRecord]()
										contracts["ProposeActionCommand"] = reflect.TypeFor[prifly.ProposeActionCommand]()
										if g.actionAdmissions {
											for old, current := range map[string]string{
												"CoreRunViewV18": "CoreRunViewV19", "CoreRunStateV18": "CoreRunStateV19",
												"CoreNextViewV18": "CoreNextViewV19", "CoreWorkflowInvocationV18": "CoreWorkflowInvocationV19",
												"CorePreviewV18": "CorePreviewV19", "CoreStepReadViewV18": "CoreStepReadViewV19",
												"CoreCapabilitiesV18": "CoreCapabilitiesV19",
											} {
												contracts[current] = contracts[old]
												delete(contracts, old)
											}
											contracts["ActionAdmission"] = reflect.TypeFor[prifly.ActionAdmission]()
											contracts["AdmitActionCommand"] = reflect.TypeFor[prifly.AdmitActionCommand]()
											if g.actionGrantAdmissions {
												for old, current := range map[string]string{
													"CoreRunViewV19": "CoreRunViewV20", "CoreRunStateV19": "CoreRunStateV20",
													"CoreNextViewV19": "CoreNextViewV20", "CoreWorkflowInvocationV19": "CoreWorkflowInvocationV20",
													"CorePreviewV19": "CorePreviewV20", "CoreStepReadViewV19": "CoreStepReadViewV20",
													"CoreCapabilitiesV19": "CoreCapabilitiesV20",
												} {
													contracts[current] = contracts[old]
													delete(contracts, old)
												}
												if g.actionDeliveries {
													for old, current := range map[string]string{"CoreRunViewV20": "CoreRunViewV21", "CoreRunStateV20": "CoreRunStateV21", "CoreNextViewV20": "CoreNextViewV21", "CoreWorkflowInvocationV20": "CoreWorkflowInvocationV21", "CorePreviewV20": "CorePreviewV21", "CoreStepReadViewV20": "CoreStepReadViewV21", "CoreCapabilitiesV20": "CoreCapabilitiesV21"} {
														contracts[current] = contracts[old]
														delete(contracts, old)
													}
													contracts["ActionDelivery"] = reflect.TypeFor[prifly.ActionDelivery]()
													if g.forks {
														for old, current := range map[string]string{"CoreRunViewV21": "CoreRunViewV22", "CoreRunStateV21": "CoreRunStateV22", "CoreNextViewV21": "CoreNextViewV22", "CoreWorkflowInvocationV21": "CoreWorkflowInvocationV22", "CorePreviewV21": "CorePreviewV22", "CoreStepReadViewV21": "CoreStepReadViewV22", "CoreCapabilitiesV21": "CoreCapabilitiesV22"} {
															contracts[current] = contracts[old]
															delete(contracts, old)
														}
														contracts["ForkProvenance"] = reflect.TypeFor[prifly.ForkProvenance]()
														if g.workspaces {
															for old, current := range map[string]string{"CoreRunViewV22": "CoreRunViewV23", "CoreRunStateV22": "CoreRunStateV23", "CoreNextViewV22": "CoreNextViewV23", "CoreWorkflowInvocationV22": "CoreWorkflowInvocationV23", "CorePreviewV22": "CorePreviewV23", "CoreStepReadViewV22": "CoreStepReadViewV23", "CoreCapabilitiesV22": "CoreCapabilitiesV23", "SessionHandoffV2": "SessionHandoffV3", "SessionTaskV2": "SessionTaskV3", "SessionSubmissionV2": "SessionSubmissionV3"} {
																contracts[current] = contracts[old]
																delete(contracts, old)
															}
															if g.workspaceTrees {
																for old, current := range map[string]string{"CoreRunViewV23": "CoreRunViewV24", "CoreRunStateV23": "CoreRunStateV24", "CoreNextViewV23": "CoreNextViewV24", "CoreWorkflowInvocationV23": "CoreWorkflowInvocationV24", "CorePreviewV23": "CorePreviewV24", "CoreStepReadViewV23": "CoreStepReadViewV24", "CoreCapabilitiesV23": "CoreCapabilitiesV24", "SessionHandoffV3": "SessionHandoffV4", "SessionTaskV3": "SessionTaskV4", "SessionSubmissionV3": "SessionSubmissionV4"} {
																	contracts[current] = contracts[old]
																	delete(contracts, old)
																}
																contracts["WorkspaceTreeManifest"] = reflect.TypeFor[prifly.WorkspaceTreeManifest]()
																contracts["WorkspaceTreeEntry"] = reflect.TypeFor[prifly.WorkspaceTreeEntry]()
																contracts["WorkspaceTreeHandoff"] = reflect.TypeFor[prifly.WorkspaceTreeHandoff]()
																contracts["WorkspaceTreeLocation"] = reflect.TypeFor[prifly.WorkspaceTreeLocation]()
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	} else if g.waits {
		contracts = map[string]reflect.Type{
			"CoreRunViewV9":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV9":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV9":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV9":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV9": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":          reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":      reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":   reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":           reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":          reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":           reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":             reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":              reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":        reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":           reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":           reflect.TypeFor[prifly.ContextRequest](),
			"SessionHandoff":           reflect.TypeFor[prifly.SessionHandoff](),
			"SessionTask":              reflect.TypeFor[prifly.SessionTask](),
			"SessionSubmission":        reflect.TypeFor[prifly.SessionSubmission](),
			"Waiver":                   reflect.TypeFor[prifly.Waiver](),
			"ParallelProgress":         reflect.TypeFor[prifly.ParallelProgress](),
			"JoinDecision":             reflect.TypeFor[prifly.JoinDecision](),
			"SealedItem":               reflect.TypeFor[prifly.SealedItem](),
			"WaitRegistration":         reflect.TypeFor[prifly.WaitRegistration](),
			"EventEnvelope":            reflect.TypeFor[prifly.EventEnvelope](),
			"InboxEvent":               reflect.TypeFor[prifly.InboxEvent](),
			"WaitProgress":             reflect.TypeFor[prifly.WaitProgress](),
		}
	} else if g.maps {
		contracts = map[string]reflect.Type{
			"CoreRunViewV8":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV8":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV8":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV8":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV8": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":          reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":      reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":   reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":           reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":          reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":           reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":             reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":              reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":        reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":           reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":           reflect.TypeFor[prifly.ContextRequest](),
			"SessionHandoff":           reflect.TypeFor[prifly.SessionHandoff](),
			"SessionTask":              reflect.TypeFor[prifly.SessionTask](),
			"SessionSubmission":        reflect.TypeFor[prifly.SessionSubmission](),
			"Waiver":                   reflect.TypeFor[prifly.Waiver](),
			"ParallelProgress":         reflect.TypeFor[prifly.ParallelProgress](),
			"JoinDecision":             reflect.TypeFor[prifly.JoinDecision](),
			"SealedItem":               reflect.TypeFor[prifly.SealedItem](),
		}
	} else if g.parallel {
		contracts = map[string]reflect.Type{
			"CoreRunViewV7":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV7":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV7":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV7":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV7": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":          reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":      reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":   reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":           reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":          reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":           reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":             reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":              reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":        reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":           reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":           reflect.TypeFor[prifly.ContextRequest](),
			"SessionHandoff":           reflect.TypeFor[prifly.SessionHandoff](),
			"SessionTask":              reflect.TypeFor[prifly.SessionTask](),
			"SessionSubmission":        reflect.TypeFor[prifly.SessionSubmission](),
			"Waiver":                   reflect.TypeFor[prifly.Waiver](),
			"ParallelProgress":         reflect.TypeFor[prifly.ParallelProgress](),
			"JoinDecision":             reflect.TypeFor[prifly.JoinDecision](),
		}
	} else if g.waivers {
		contracts = map[string]reflect.Type{
			"CoreRunViewV6":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV6":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV6":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV6":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV6": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":          reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":      reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":   reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":           reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":          reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":           reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":             reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":              reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":        reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":           reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":           reflect.TypeFor[prifly.ContextRequest](),
			"SessionHandoff":           reflect.TypeFor[prifly.SessionHandoff](),
			"SessionTask":              reflect.TypeFor[prifly.SessionTask](),
			"SessionSubmission":        reflect.TypeFor[prifly.SessionSubmission](),
			"Waiver":                   reflect.TypeFor[prifly.Waiver](),
		}
	} else if g.sessions {
		contracts = map[string]reflect.Type{
			"CoreRunViewV5":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV5":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV5":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV5":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV5": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":          reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":      reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":   reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":           reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":          reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":           reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":             reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":              reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":        reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":           reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":           reflect.TypeFor[prifly.ContextRequest](),
			"SessionHandoff":           reflect.TypeFor[prifly.SessionHandoff](),
			"SessionTask":              reflect.TypeFor[prifly.SessionTask](),
			"SessionSubmission":        reflect.TypeFor[prifly.SessionSubmission](),
		}
	} else if g.contexts {
		contracts = map[string]reflect.Type{
			"CoreRunViewV4":            reflect.TypeFor[prifly.RunView](),
			"CoreRunStateV4":           reflect.TypeFor[prifly.Run](),
			"CoreNextViewV4":           reflect.TypeFor[prifly.NextView](),
			"CorePreviewV4":            reflect.TypeFor[prifly.Preview](),
			"CoreWorkflowInvocationV4": reflect.TypeFor[prifly.Invocation](),
			"LocalRegistryV3":          reflect.TypeFor[prifly.RegistryFile](),
			"CoreConfigurationV2":      reflect.TypeFor[prifly.Configuration](),
			"LocalContextManifestV2":   reflect.TypeFor[prifly.ContextManifest](),
			"ContextProfile":           reflect.TypeFor[prifly.ContextProfile](),
			"CheckDefinition":          reflect.TypeFor[flow.CheckDefinition](),
			"CheckExecution":           reflect.TypeFor[prifly.CheckExecution](),
			"CheckRequest":             reflect.TypeFor[prifly.CheckRequest](),
			"CheckResult":              reflect.TypeFor[prifly.CheckResult](),
			"PendingAcceptance":        reflect.TypeFor[prifly.PendingAcceptance](),
			"SourceSnapshot":           reflect.TypeFor[prifly.SourceSnapshot](),
			"ContextRequest":           reflect.TypeFor[prifly.ContextRequest](),
		}
	}
	if g.decisionState {
		for old, current := range map[string]string{"CoreRunViewV24": "CoreRunViewV25", "CoreRunStateV24": "CoreRunStateV25", "CoreNextViewV24": "CoreNextViewV25", "CoreWorkflowInvocationV24": "CoreWorkflowInvocationV25", "CorePreviewV24": "CorePreviewV25", "CoreStepReadViewV24": "CoreStepReadViewV25", "CoreCapabilitiesV24": "CoreCapabilitiesV25", "SessionHandoffV4": "SessionHandoffV5", "SessionTaskV4": "SessionTaskV5", "SessionSubmissionV4": "SessionSubmissionV5"} {
			contracts[current] = contracts[old]
			delete(contracts, old)
		}
		contracts["DecisionCatalog"] = reflect.TypeFor[prifly.DecisionCatalog]()
		contracts["DecisionDefinition"] = reflect.TypeFor[prifly.DecisionDefinition]()
		contracts["DecisionSheet"] = reflect.TypeFor[prifly.DecisionSheet]()
		contracts["DecisionRecord"] = reflect.TypeFor[prifly.DecisionRecord]()
	}
	names := make([]string, 0, len(contracts))
	for name, t := range contracts {
		g.defs[name] = g.schema(t)
		names = append(names, name)
	}
	sort.Strings(names)
	for def, version := range map[string]string{"runtime_RunView": prifly.ReadVersion, "runtime_Run": prifly.StateVersion, "runtime_NextView": "foundation-next/1", "runtime_Preview": "foundation-preview/1", "runtime_StepReadView": "foundation-step-read/1", "runtime_PublishCommand": "1", "runtime_TelemetryQuery": prifly.TelemetryQueryVersion, "runtime_TelemetryResponse": prifly.TelemetryReportVersion, "runtime_TelemetrySampleData": "telemetry-sample/1", "runtime_ContextManifest": "local-context/1", "runtime_TimingTree": "1"} {
		if _, exists := g.defs[def]; exists {
			g.property(def, "schema_version", map[string]any{"const": version})
		}
	}
	g.property("runtime_Run", "semantics_profile", map[string]any{"const": "foundation-sequence/1"})
	g.property("runtime_Run", "status", enum("ready", "running", "waiting", "stopping", "completed", "failed", "cancelled", "uncertain"))
	g.property("runtime_Run", "outcome", nullable(enum("succeeded", "rejected", "no_work")))
	g.property("runtime_Duration", "quality", enum("measured", "estimated", "partial", "unavailable", "not_applicable"))
	g.property("runtime_Preview", "admission", map[string]any{"const": false})
	for _, described := range []struct{ def, field, text string }{
		{"runtime_WorkspaceTreeLocation", "path", "Workspace-relative path of the tree this port is captured from. An exact_file policy admits only the declared capture path and the location may be omitted for it; a direct-child policy needs the chosen child name here."},
		{"runtime_WorkspaceTreeHandoff", "input_location", "Where the runtime already materialized this port's input tree. A submission may repeat this path or omit it, but may not name another."},
		{"runtime_SessionSubmission", "result", "The StepResult document itself. It is validated against the step's own result contract, not by this schema: read that contract with prifly schema StepResult."},
	} {
		g.describe(described.def, described.field, described.text)
	}
	bundle := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:prifly:foundation-public:1", "title": "Pri-Fly foundation public DTO contracts", "description": "Generated shapes plus explicit F1 versions. Payload, graph, authority, quotas and cross-field semantic checks additionally run in the core. Baseline v1 schemas remain unchanged.", "x-prifly-contracts": names, "$defs": g.defs}
	if g.core {
		for def, version := range map[string]string{"runtime_RunView": prifly.CoreReadVersion, "runtime_Run": prifly.CoreStateVersion, "runtime_Preview": "core-preview/1", "runtime_CapabilityManifest": "capabilities/1", "runtime_EffectiveConfiguration": "effective-configuration/1"} {
			if _, exists := g.defs[def]; exists {
				g.property(def, "schema_version", map[string]any{"const": version})
			}
		}
		for _, def := range []string{"runtime_Run", "runtime_Preview"} {
			g.property(def, "semantics_profile", map[string]any{"const": flow.CoreProfile})
			g.property(def, "effective_configuration", map[string]any{"$ref": "#/$defs/runtime_EffectiveConfiguration"})
			shape := g.defs[def].(map[string]any)
			required := append(shape["required"].([]string), "effective_configuration")
			sort.Strings(required)
			shape["required"] = required
		}
		if _, exists := g.defs["runtime_ProfileCapabilities"]; exists {
			g.property("runtime_ProfileCapabilities", "semantics_profile", enum(flow.Profile, flow.CoreProfile))
		}
		g.property("runtime_EffectiveConfiguration", "inputs", map[string]any{"type": "object", "additionalProperties": map[string]any{"$ref": "#/$defs/runtime_ConfigurationValue"}})
		g.property("runtime_ConfigurationValue", "source", enum("package_default", "project", "run", "absent"))
		g.defs["runtime_ConfigurationValue"].(map[string]any)["oneOf"] = []any{
			map[string]any{"properties": map[string]any{"source": map[string]any{"const": "absent"}}, "not": map[string]any{"required": []string{"value"}}},
			map[string]any{"properties": map[string]any{"source": enum("package_default", "project", "run")}, "required": []string{"value"}},
		}
		// Reuse the actual admission schemas instead of maintaining a weaker
		// second set of configuration/projection constraints.
		builtins, _, err := prifly.Builtins()
		if err != nil {
			panic(err)
		}
		for name, id := range map[string]string{"runtime_Configuration": "core:schema/core-configuration", "runtime_ProjectionManifest": "core:schema/json-projection"} {
			if _, exists := g.defs[name]; !exists {
				continue
			}
			var shape map[string]any
			for _, definition := range builtins {
				if definition.Ref.ID == id {
					if err := json.Unmarshal(definition.Bytes, &shape); err != nil {
						panic(err)
					}
					break
				}
			}
			if shape == nil {
				panic("builtin schema is missing: " + id)
			}
			g.defs[name] = shape
		}
		bundle["$id"] = "urn:prifly:core-public:1"
		bundle["title"] = "Pri-Fly core workflow public DTO contracts"
		bundle["description"] = "Generated shapes for core-workflow/1 with explicit versions and pinned configuration. Payload, graph, authority, quotas and cross-field semantic checks additionally run in the core. Foundation and baseline v1 schemas remain unchanged."
		if g.invocations {
			invocationConstraints(&g)
			bundle["$id"] = "urn:prifly:core-invocation-public:2"
			bundle["title"] = "Pri-Fly core invocation public DTO contracts"
			bundle["description"] = "Scoped invocation state/read version 2, child linkage, per-invocation frontiers and local author registry. Legacy foundation, core version 1 and baseline WorkflowInvocation contracts remain unchanged. Tree ownership, pinned closure, control and budget invariants additionally run in the core."
		}
		if g.repeats {
			repeatConstraints(&g)
			bundle["$id"] = "urn:prifly:core-repeat-public:3"
			bundle["title"] = "Pri-Fly bounded repeat public DTO contracts"
			bundle["description"] = "Invocation state/read version 3 adds a bounded repeat frontier, exact business iteration identities and the latest durable decision. Earlier decisions remain in the journal. Delivered foundation, core version 1, choice and invocation version 2 bundles stay unchanged; ownership, pinned limits and atomic admission are additionally checked by the runtime."
		}
		if g.contexts {
			contextConstraints(&g)
			bundle["$id"] = "urn:prifly:core-context-public:4"
			bundle["title"] = "Pri-Fly context and check public DTO contracts"
			bundle["description"] = "Context state/read version 4, typed resources, local context transport version 2, source snapshots and check protocols. Shapes do not qualify execution capability, semantic isolation, evidence authority or a product acceptance gate. Exact ownership, data integrity, pinned limits and cross-field checks remain in runtime admission. All five delivered public bundles remain unchanged."
		}
		if g.sessions {
			sessionConstraints(&g)
			bundle["$id"] = "urn:prifly:core-session-public:5"
			bundle["title"] = "Pri-Fly assisted session public DTO contracts"
			bundle["description"] = "Session state/read version 5 records assisted execution: which authenticated principal holds an attempt, the pinned skill references it was handed, its claimed worktree and whether the host ever reported. A missing report is its own state and never an outcome. Shapes do not qualify host isolation, provider identity or a product acceptance gate; identity, claim ownership, deadlines and current controls remain runtime admission checks. All six delivered public bundles remain unchanged."
		}
		if g.waivers {
			waiverConstraints(&g)
			bundle["$id"] = "urn:prifly:core-waiver-public:6"
			bundle["title"] = "Pri-Fly quality waiver public DTO contracts"
			bundle["description"] = "Waiver state/read version 6 records an explicit refusal to require one named quality check. A waiver is not a pass and produces no missing artifact: the check stays unpassed and the reduction stays visible as completed_with_waivers. Meaningfulness checks are outside this reach entirely. Shapes do not qualify who may waive, which reductions are acceptable or a product acceptance gate; approver access, protected classes and expiry remain runtime admission checks. All seven delivered public bundles remain unchanged."
		}
		if g.guards {
			mapConstraints(&g)
			waitConstraints(&g)
			guardConstraints(&g)
			bundle["$id"] = "urn:prifly:core-guard-public:10"
			bundle["title"] = "Pri-Fly live guard public DTO contracts"
			bundle["description"] = "Guard state/read version 10 records the live rules one Run was registered with and the observations they were decided on. A guard reads only facts the Run already holds - its inputs and completed stage outputs - and watches nothing outside it. Logic is three-valued: true, false and unknown, where missing, null and false stay three different answers, and a type or read fault is recorded as an error rather than folded into unknown. A start guard gates an activation the graph has already chosen and never creates work; once the activation is open, the guard turning false does not take it back. A stop guard declares both its action and its reaction to unknown, and both refuse new ordinary work: there is no implicit fail-open. Firing creates the ordinary durable stop, released only by the ordinary explicit release; recovered facts never lift it by themselves. A true observed while the cursor is behind it is latched and cannot be erased by a later false, and while unprocessed observations exist the guarded scope admits nothing. Shapes do not qualify external sources, freshness, timers or a product acceptance gate; scope ownership, budgets and pinned definitions remain runtime admission checks. All ten delivered public bundles remain unchanged."
		}
		if g.reportedCosts {
			reportedCostConstraints(&g)
			bundle["$id"] = "urn:prifly:core-reported-cost-public:11"
			bundle["title"] = "Pri-Fly named reported cost public DTO contracts"
			bundle["description"] = "Reported-cost state/read version 11 records exact non-negative decimal amounts that named sources claimed for one Attempt. Absence is unobserved, an explicit zero remains zero, and reports from different sources remain separate. Pri-Fly does not infer a price from usage, reconcile reports, convert currencies or qualify a provider bill. The assisted host principal and authority observation retain who transmitted the claims and when. Shapes do not add provider usage, rate cards, budgets, aggregation, late adjustments or a product acceptance gate. All twelve delivered public bundles remain unchanged."
		}
		if g.artifacts {
			artifactPublicationConstraints(&g)
			bundle["$id"] = "urn:prifly:core-artifact-publication:12"
			bundle["title"] = "Pri-Fly early artifact publication public DTO contracts"
			bundle["description"] = "Artifact-publication state/read version 12 records a typed logical item only after the authority has copied and sealed the exact candidate bytes. Its ArtifactRef is distinct from the producer workspace path and from the eventual final StepResult. Exact command and logical-key retries retain the first accepted record without rereading a changed candidate. This slice does not add subscriptions, close manifests or executable per-item checks. All thirteen delivered public bundles remain unchanged."
			if g.closures {
				artifactClosureConstraints(&g)
				bundle["$id"] = "urn:prifly:core-artifact-closure:13"
				bundle["title"] = "Pri-Fly artifact closure public DTO contracts"
				bundle["description"] = "Artifact-closure state/read version 13 records one authority-sealed exact manifest for a keyed-many artifact hook. Close requires the full ordered membership, is idempotent by logical hook identity, and forbids later items without settling the producer. This slice does not add subscription cursors, stream assignments or executable per-item checks. All fourteen delivered public bundles remain unchanged."
				if g.subscriptions {
					publicationSubscriptionConstraints(&g)
					bundle["$id"] = "urn:prifly:core-publication-subscription:14"
					bundle["title"] = "Pri-Fly publication subscription public DTO contracts"
					bundle["description"] = "Publication-subscription state/read version 14 records an independent durable cursor and assignment ledger for each bounded repeat subscriber. Item, Closed and Interrupted deliveries remain distinct; a pending item stays pinned until its repeat body settles. Earlier bundles remain unchanged."
					if g.publicationChecks {
						publicationChecksConstraints(&g)
						bundle["$id"] = "urn:prifly:core-publication-checks:15"
						bundle["title"] = "Pri-Fly checked artifact publication public DTO contracts"
						bundle["description"] = "Checked artifact-publication state/read version 15 retains one sealed candidate while each declared per-item content check runs. The item becomes visible only with all passing evidence; pending candidates are not deliveries. Earlier bundles remain unchanged."
						if g.publicationNewOnly {
							publicationNewOnlyConstraints(&g)
							bundle["$id"] = "urn:prifly:core-publication-new-only:16"
							bundle["title"] = "Pri-Fly new-only publication subscription public DTO contracts"
							bundle["description"] = "New-only publication state/read version 16 records the authority event cut at which each publication wait or stream subscription began. Retained sources remain unchanged; a new-only source may consume only publications and closures committed after its durable cut. Earlier bundles remain unchanged."
							if g.publicationFailure {
								publicationFailureConstraints(&g)
								bundle["$id"] = "urn:prifly:core-publication-terminal-failure:17"
								bundle["title"] = "Pri-Fly terminal producer-failure publication contracts"
								bundle["description"] = "Publication state/read version 17 records an explicit terminal producer failure as an interruption, rather than pretending it was deadline expiry. Only source versions that opt into this policy take that route; earlier bundles remain unchanged."
								if g.actionIntents {
									actionIntentConstraints(&g)
									bundle["$id"] = "urn:prifly:core-action-intent:18"
									bundle["title"] = "Pri-Fly durable ActionIntent proposal contracts"
									bundle["description"] = "Action-intent state/read version 18 records one exact tool operation proposed by the current assisted host, including its canonical digest and a copied sealed ToolDescriptor. Proposal is not an Action Admission, delivery or external effect; it consumes no approval or grant and never dispatches an adapter. Earlier bundles remain unchanged."
									if g.actionAdmissions {
										actionAdmissionConstraints(&g)
										bundle["$id"] = "urn:prifly:core-action-admission:19"
										bundle["title"] = "Pri-Fly durable ActionAdmission contracts"
										bundle["description"] = "Action-admission state/read version 19 records one authority decision for a sealed ActionIntent and the exact control approvals consumed with it in the same SQLite transaction. It does not consume grants, create an ActionDelivery, dispatch an adapter or claim an external effect. Earlier bundles remain unchanged."
										if g.actionGrantAdmissions {
											actionGrantAdmissionConstraints(&g)
											bundle["$id"] = "urn:prifly:core-action-grant-admission:20"
											bundle["title"] = "Pri-Fly exact resource-scoped ActionGrantAdmission contracts"
											bundle["description"] = "Action-grant-admission state/read version 20 records one exact resource-scoped Grant consumed atomically with any required approval and the ActionAdmission for a sealed ActionIntent. It does not create an ActionDelivery, dispatch an adapter or claim an external effect. Version 19 remains approval-only."
											if g.actionDeliveries {
												actionDeliveryConstraints(&g)
												bundle["$id"] = "urn:prifly:core-action-delivery:21"
												bundle["title"] = "Pri-Fly prepared ActionDelivery contracts"
												bundle["description"] = "Action-delivery state/read version 21 records one prepared delivery before any adapter call. Prepared means not_started; no credential, dispatch, receipt or external effect exists. Version 20 remains admission-only."
												if g.forks {
													forkConstraints(&g)
													bundle["$id"] = "urn:prifly:core-fork:22"
													bundle["title"] = "Pri-Fly linked Run fork contracts"
													bundle["description"] = "Fork state/read version 22 records a new Core Run linked to one exact source Run version. It preserves the source unchanged, copies no approval, mutable worker state or action safety, and permits only explicit source outputs that are separately declared as new inputs. It does not add general trusted reuse, cross-project sharing or a transfer of external-effect authority."
													if g.workspaces {
														workspaceConstraints(&g)
														bundle["$id"] = "urn:prifly:core-workspace:23"
														bundle["title"] = "Pri-Fly declared repository workspace contracts"
														bundle["description"] = "Workspace state/read version 23 records the exact worktree or explicitly selected checkout handed to an assisted workspace-write host. It keeps authority scratch separate from repository files and does not create a provider, switch branches, reset, clean or remove a checkout. Earlier worktree-only session contracts remain unchanged."
														if g.workspaceTrees {
															workspaceTreeConstraints(&g)
															bundle["$id"] = "urn:prifly:core-workspace-tree:24"
															bundle["title"] = "Pri-Fly workspace tree artifact contracts"
															bundle["description"] = "Workspace-tree state/read version 24 records a declared, bounded native file manifest for an assisted repository workspace. The host reports a confined location; the runtime seals the exact files and owns all artifact references. Earlier session and workspace contracts remain unchanged."
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
		if g.decisionState {
			decisionStateConstraints(&g)
			bundle["$id"] = "urn:prifly:core-decisions:25"
			bundle["title"] = "Pri-Fly Run decision contracts"
			bundle["description"] = "Decision state/read version 25 seals a declared catalog and its answered preflight sheet with one Run. The catalog names values only; it grants no authority, changes no graph and cannot capture an executor's undeclared native dialog. Earlier bundles remain unchanged."
		}
		if g.waits && !g.guards {
			mapConstraints(&g)
			waitConstraints(&g)
			bundle["$id"] = "urn:prifly:core-wait-public:9"
			bundle["title"] = "Pri-Fly wait registration public DTO contracts"
			bundle["description"] = "Wait state/read version 9 records a durable promise that one exact signal may resolve one exact wait, and an inbox of what actually arrived. A registration is reserved before it is active, and a reserved one routes nothing. A wait resolves exactly once, on an event or on expiry, and expiry creates no event: a timeout is the absence of a signal, never a manufactured one. A refused delivery is kept with its reason, because never arriving and arriving unusable are different answers. The nonce correlates a signal to a wait and is not authentication; who may speak is the source adapter's question. Received time is the authority's and is what deadlines are judged by. Shapes do not qualify durable wakeup, timer ownership or a product acceptance gate; correlation, generation, caps and pinned definitions remain runtime admission checks. All nine delivered public bundles remain unchanged."
		}
		if g.maps && !g.waits && !g.guards {
			mapConstraints(&g)
			bundle["$id"] = "urn:prifly:core-map-public:8"
			bundle["title"] = "Pri-Fly sealed collection public DTO contracts"
			bundle["description"] = "Map state/read version 8 records a fan-out whose branches were derived from a collection sealed before anything was admitted: one entry per item, each carrying its typed identity, its position in the sealed collection and the artifact cut from it. A position is evidence about the collection, never a business identity, and the number 1 is not the string \"1\". An empty collection seals nothing and takes its own declared route rather than a join verdict about no branches. Shapes do not qualify simultaneity, item isolation or a product acceptance gate; the sealed identities, the declared item cap, budgets and pinned definitions remain runtime admission checks. All eight delivered public bundles remain unchanged."
		}
		if g.parallel && !g.maps {
			parallelConstraints(&g)
			bundle["$id"] = "urn:prifly:core-parallel-public:7"
			bundle["title"] = "Pri-Fly branch fan-out public DTO contracts"
			bundle["description"] = "Parallel state/read version 7 records a fan-out as ordinary branch invocations and one durable join decision per settled branch. A satisfied join is not a successful outcome and a decision is evidence about one exact branch, never an instruction to run it again. This profile enters the declared branches one at a time, so a branch left out of a decided join was never entered rather than cancelled, and that disposition is recorded. Shapes do not qualify simultaneity, branch isolation or a product acceptance gate; the sealed branch order, budgets and pinned definitions remain runtime admission checks. All eight delivered public bundles remain unchanged."
		}
	} else {
		g.property("runtime_TelemetryQuery", "mode", enum("catalog", "records", "aggregate"))
		g.property("runtime_TelemetryQuery", "limit", map[string]any{"type": "integer", "minimum": 1, "maximum": 1000})
		g.property("runtime_TelemetryQuery", "cut", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
		g.property("runtime_PublishCommand", "kind", enum("state", "event"))
		// Value became omitempty only so the v2 artifact variant can forbid it.
		// The delivered v1 state/event command still requires it byte-for-byte.
		publish := g.defs["runtime_PublishCommand"].(map[string]any)
		required := append(publish["required"].([]string), "value")
		sort.Strings(required)
		publish["required"] = required
		g.property("runtime_PublishCommand", "expected_state_version", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
		g.defs["runtime_PublishCommand"].(map[string]any)["oneOf"] = []any{
			map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "state"}}, "required": []string{"expected_state_version"}, "not": map[string]any{"required": []string{"event_key"}}},
			map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "event"}, "event_key": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:/-]{0,127}$"}}, "required": []string{"event_key"}, "not": map[string]any{"required": []string{"expected_state_version"}}},
		}
		g.property("runtime_NextView", "admission", map[string]any{"const": false})
		g.property("runtime_NextView", "read_only", map[string]any{"const": true})
	}
	emit(bundle)
}

func emit(bundle map[string]any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bundle); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Choice decisions have their own contract; extending the delivered core Run
// bundle would silently change its meaning for P2-01 readers.
func choiceSchema() map[string]any {
	g := generator{defs: map[string]any{}, core: true}
	g.defs["ChoiceDecision"] = g.schema(reflect.TypeFor[prifly.ChoiceDecision]())
	for _, name := range []string{"FieldRef", "ImmutableRef", "ArtifactRef", "Timestamp"} {
		data, err := flow.ProtocolSchema(name)
		if err != nil {
			panic(err)
		}
		var source struct {
			Defs map[string]any `json:"$defs"`
		}
		if err := json.Unmarshal(data, &source); err != nil {
			panic(err)
		}
		for key, value := range source.Defs {
			g.defs[key] = value
		}
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	g.defs["flow_Ref"] = ref("ImmutableRef")
	g.defs["runtime_ArtifactRef"] = ref("ArtifactRef")
	// iteration_output is not part of P2-02. Keep the original schema intact.
	field := g.defs["FieldRef"].(map[string]any)
	field["oneOf"] = field["oneOf"].([]any)[:2]
	g.defs["flow_FieldRef"] = ref("FieldRef")
	g.property("runtime_ChoiceDecision", "schema_version", map[string]any{"const": prifly.ChoiceDecisionVersion})
	for _, name := range []string{"id", "run_id", "workflow_invocation_id", "stage_activation_id", "stage_id", "branch_id", "next_stage_id", "failure"} {
		g.property("runtime_ChoiceDecision", name, ref("Identifier"))
	}
	g.property("runtime_ChoiceDecision", "failure_path", ref("Pointer"))
	g.property("runtime_ChoiceDecision", "selection", enum("exclusive", "first_match"))
	g.property("runtime_ChoiceDecision", "route", enum("branch", "default", "on_unknown", "on_error", "failed"))
	g.property("runtime_ChoiceDecision", "evaluations", map[string]any{"type": "array", "items": ref("runtime_ChoiceEvaluation"), "minItems": 1, "maxItems": 64})
	g.property("runtime_ChoiceDecision", "inputs", map[string]any{"type": "array", "items": ref("runtime_ChoiceInput"), "maxItems": 512})
	g.property("runtime_ChoiceEvaluation", "branch_id", ref("Identifier"))
	g.property("runtime_ChoiceEvaluation", "result", enum("true", "false", "unknown", "error", "not_evaluated"))
	g.property("runtime_ChoiceInput", "source_ref", ref("ArtifactRef"))
	g.property("runtime_ChoiceInput", "producer_activation_id", ref("Identifier"))
	g.property("runtime_ChoiceInput", "availability", enum("present", "absent", "unavailable"))
	g.defs["runtime_ChoiceInput"].(map[string]any)["if"] = map[string]any{"properties": map[string]any{"availability": map[string]any{"const": "present"}}}
	g.defs["runtime_ChoiceInput"].(map[string]any)["then"] = map[string]any{"required": []string{"source_ref"}}
	g.property("runtime_Observation", "utc", ref("Timestamp"))
	g.property("runtime_Observation", "session", ref("Identifier"))
	g.property("runtime_Observation", "monotonic_ms", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	variants := []any{}
	for _, route := range []string{"branch", "default", "on_unknown", "on_error", "failed"} {
		variant := map[string]any{"properties": map[string]any{"route": map[string]any{"const": route}}}
		required, forbidden := []string{}, []any{}
		for _, name := range []string{"branch_id", "next_stage_id", "failure", "failure_path"} {
			allowed := name == "branch_id" && route == "branch" || name == "next_stage_id" && route != "failed" || (name == "failure" || name == "failure_path") && (route == "on_error" || route == "failed")
			if allowed && name != "failure_path" {
				required = append(required, name)
			} else if !allowed {
				forbidden = append(forbidden, map[string]any{"required": []string{name}})
			}
		}
		variant["required"] = required
		variant["not"] = map[string]any{"anyOf": forbidden}
		if route == "on_error" {
			variant["properties"].(map[string]any)["failure"] = map[string]any{"not": enum("condition_unknown", "no_transition")}
		}
		variants = append(variants, variant)
	}
	g.defs["runtime_ChoiceDecision"].(map[string]any)["oneOf"] = variants
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:prifly:choice-decision:1", "title": "Pri-Fly choice decision", "description": "Core decision event payload with ordered branch trace and exact input references. Route/activation ownership and atomic commit are enforced by the runtime.", "x-prifly-contracts": []string{"ChoiceDecision"}, "$ref": "#/$defs/ChoiceDecision", "$defs": g.defs}
}
