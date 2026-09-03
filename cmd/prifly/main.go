package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/release"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func main() {
	if handled, code := flow.SchemaWorker(os.Args[1:], os.Stdin, os.Stdout); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(execute(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

type cli struct {
	project, format string
	out             io.Writer
}

var updateBinary = func(ctx context.Context, version string) (release.Result, error) {
	return release.DefaultUpdater(version).Update(ctx)
}

type usageError string

func (e usageError) Error() string { return string(e) }
func commandID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "command:" + hex.EncodeToString(b[:])
}

func execute(ctx context.Context, args []string, out, errout io.Writer) int {
	c := cli{project: ".", format: "text", out: out}
	args, err := c.globals(args)
	if err == nil {
		err = c.run(ctx, args)
	}
	if err == nil {
		return 0
	}
	p, exit := makeProblem(err)
	enc := json.NewEncoder(errout)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(p)
	return exit
}
func makeProblem(err error) (prifly.Problem, int) {
	var input usageError
	if errors.As(err, &input) {
		return prifly.ProblemFor(&flow.Problem{Code: "invalid_usage", Message: string(input)})
	}
	return prifly.ProblemFor(err)
}

func (c *cli) globals(args []string) ([]string, error) {
	rest := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			c.format = "json"
		case a == "--project":
			if i+1 == len(args) {
				return nil, usageError("--project needs a directory")
			}
			i++
			c.project = args[i]
		case strings.HasPrefix(a, "--project="):
			c.project = strings.TrimPrefix(a, "--project=")
		case a == "--format":
			if i+1 == len(args) {
				return nil, usageError("--format needs text, json or csv")
			}
			i++
			c.format = args[i]
		case strings.HasPrefix(a, "--format="):
			c.format = strings.TrimPrefix(a, "--format=")
		default:
			rest = append(rest, a)
		}
	}
	if c.project == "" || c.format != "text" && c.format != "json" && c.format != "csv" {
		return nil, usageError("Use --project DIR and --format text|json|csv")
	}
	return rest, nil
}
func flags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	return f
}
func parse(f *flag.FlagSet, args []string) error {
	if err := f.Parse(args); err != nil {
		return usageError("Invalid flags for " + f.Name() + "; run prifly help")
	}
	if f.NArg() != 0 {
		return usageError("Unexpected arguments for " + f.Name())
	}
	return nil
}
func (c *cli) emit(value any) error {
	if c.format == "csv" {
		return writeCSV(c.out, value)
	}
	if c.format == "text" {
		if view, ok := value.(prifly.RunView); ok {
			return renderRun(c.out, view)
		}
	}
	enc := json.NewEncoder(c.out)
	enc.SetEscapeHTML(true)
	if c.format == "text" {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(value)
}
func (c *cli) run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(c.out, help)
		return err
	}
	switch args[0] {
	case "update":
		if len(args) != 1 {
			return usageError("update takes no arguments")
		}
		result, err := updateBinary(ctx, prifly.Version)
		if err != nil {
			return err
		}
		return c.emit(result)
	case "version":
		if len(args) != 1 {
			return usageError("version takes no arguments")
		}
		return c.emit(map[string]any{"schema_version": "foundation-version/1", "version": prifly.Version, "semantics_profile": flow.Profile, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH})
	case "monitor":
		return c.monitor(ctx, c.project, args[1:])
	case "init":
		f := flags("init")
		profile := f.String("profile", flow.Profile, "explicit execution semantics profile")
		if err := f.Parse(args[1:]); err != nil {
			return usageError("Invalid flags for init; run prifly help")
		}
		if f.NArg() > 1 {
			return usageError("init takes at most one directory")
		}
		root := c.project
		if f.NArg() == 1 {
			root = f.Arg(0)
		}
		if err := prifly.InitProfile(root, *profile); err != nil {
			return err
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-init/1", "project": absolute, "initialized": true, "installed_packages": 0, "network_used": false})
	case "capabilities":
		if len(args) != 1 {
			return usageError("capabilities takes no arguments")
		}
		return c.emit(prifly.Capabilities())
	case "ref":
		return c.ref(args[1:])
	case "schema":
		if len(args) != 2 {
			return usageError("schema requires an exact protocol contract name")
		}
		b, err := prifly.PublicSchema(args[1])
		if err != nil {
			b, err = flow.ProtocolSchema(args[1])
		}
		if err != nil {
			return err
		}
		return c.emit(json.RawMessage(b))
	case "step":
		return c.step(ctx, args[1:])
	case "project":
		return c.projectCommand(ctx, args[1:])
	}
	readOnly := true
	if args[0] == "run" && len(args) > 1 {
		switch args[1] {
		case "start", "fork", "drive", "pause", "cancel", "stop", "release", "resume", "waive":
			readOnly = false
		}
		if args[1] == "decision" && len(args) > 3 && (args[3] == "request" || args[3] == "answer") {
			readOnly = false
		}
	}
	if args[0] == "control" && len(args) > 1 && (args[1] == "stop" || args[1] == "release") {
		readOnly = false
	}
	if args[0] == "package" && len(args) > 1 && (args[1] == "import" || args[1] == "remove" || args[1] == "quarantine" || args[1] == "revoke" || args[1] == "restore" || args[1] == "trust-root") {
		readOnly = false
	}
	if args[0] == "claim" && len(args) > 1 && (args[1] == "create" || args[1] == "release" || args[1] == "heartbeat") {
		readOnly = false
	}
	if args[0] == "session" && len(args) > 1 && (args[1] == "publish" || args[1] == "action" || args[1] == "submit" || args[1] == "disconnect") {
		readOnly = false
	}
	if args[0] == "action" && len(args) > 1 && (args[1] == "propose" || args[1] == "admit") {
		readOnly = false
	}
	if args[0] == "approval" && len(args) > 1 && args[1] != "list" {
		readOnly = false
	}
	if args[0] == "grant" && len(args) > 1 && args[1] != "list" {
		readOnly = false
	}
	if args[0] == "capacity" && len(args) > 1 && args[1] == "set" {
		readOnly = false
	}
	if args[0] == "artifact" && len(args) > 1 && (args[1] == "import" || args[1] == "export") {
		readOnly = false
	}
	if args[0] == "source" && len(args) > 1 && args[1] == "import" {
		readOnly = false
	}
	if args[0] == "task" && len(args) > 1 && args[1] == "prepare" {
		readOnly = false
	}
	e, err := prifly.Open(c.project, readOnly)
	if err != nil {
		return err
	}
	defer e.Close()
	switch args[0] {
	case "doctor":
		if len(args) != 1 {
			return usageError("doctor takes no arguments")
		}
		result, err := e.Check(ctx)
		if err != nil {
			return err
		}
		return c.emit(result)
	case "inventory":
		if len(args) != 1 {
			return usageError("inventory takes no arguments")
		}
		defs, _, err := e.Inventory()
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-inventory/1", "definitions": defs})
	case "validate", "preview":
		f := flags(args[0])
		path := f.String("workflow", "", "explicit workflow file")
		brief := f.String("brief", "", "optional explicit brief to preview")
		refFiles := bindings{}
		f.Var(refFiles, "input-ref", "explicit immutable input port=REF.json")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *path == "" {
			return usageError("--workflow is required")
		}
		var refs map[string]prifly.ArtifactRef
		if len(refFiles) > 0 {
			refs = map[string]prifly.ArtifactRef{}
			for port, file := range refFiles {
				var ref prifly.ArtifactRef
				if err := readJSON(c.requestFile(file), &ref); err != nil {
					return err
				}
				refs[port] = ref
			}
		}
		result, err := e.Preview(prifly.PreviewOptions{WorkflowFile: *path, BriefFile: *brief, InputRefs: refs})
		if err != nil {
			return err
		}
		return c.emit(result)
	case "run":
		return c.runCommand(ctx, e, args[1:])
	case "control":
		return c.control(ctx, e, args[1:])
	case "package":
		return c.packages(ctx, e, args[1:])
	case "claim":
		return c.claims(ctx, e, args[1:])
	case "session":
		return c.session(ctx, e, args[1:])
	case "action":
		return c.action(ctx, e, args[1:])
	case "approval":
		return c.approval(ctx, e, args[1:])
	case "grant":
		return c.grant(ctx, e, args[1:])
	case "capacity":
		return c.capacity(ctx, e, args[1:])
	case "artifact":
		return c.artifact(e, args[1:])
	case "source":
		return c.source(e, args[1:])
	case "task":
		return c.task(e, args[1:])
	case "telemetry":
		return c.telemetry(ctx, e, args[1:])
	case "command":
		if len(args) < 2 || args[1] != "receipt" {
			return usageError("command receipt --id COMMAND_ID")
		}
		f := flags("command receipt")
		id := f.String("id", "", "")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *id == "" {
			return usageError("command receipt requires --id")
		}
		receipt, err := e.Receipt(ctx, *id)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-receipt/1", "receipt": receipt})
	default:
		return usageError("Unsupported operation. Run prifly help for the foundation command set.")
	}
}

// action exposes the two durable parts of a managed operation. Both commands
// stop before delivery, so no adapter or external target is contacted here.
func (c *cli) action(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("action requires propose|admit")
	}
	f := flags("action " + args[0])
	file := f.String("file", "", "")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if *file == "" {
		return usageError("action " + args[0] + " requires --file COMMAND.json")
	}
	data, err := readFile(*file, prifly.MaxDefinitionBytes)
	if err != nil {
		return err
	}
	switch args[0] {
	case "propose":
		command, err := prifly.ParseProposeActionCommand(data)
		if err != nil {
			return usageError("Invalid closed action proposal")
		}
		result, err := e.ProposeSessionAction(ctx, command)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "admit":
		command, err := prifly.ParseAdmitActionCommand(data)
		if err != nil {
			return usageError("Invalid closed action admission")
		}
		result, err := e.AdmitSessionAction(ctx, command)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	default:
		return usageError("action requires propose|admit")
	}
}

type bindings map[string]string

func (b bindings) String() string { return "port=path" }
func (b bindings) Set(s string) error {
	port, path, ok := strings.Cut(s, "=")
	if !ok || port == "" || path == "" {
		return errors.New("expected port=path")
	}
	if _, exists := b[port]; exists {
		return errors.New("duplicate input")
	}
	b[port] = path
	return nil
}
func (c *cli) runCommand(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("run requires start|fork|status|next|explain|events|timing|drive|decision|decisions|pause|cancel|release|resume")
	}
	if args[0] == "start" {
		f := flags("run start")
		workflow := f.String("workflow", "", "")
		brief := f.String("brief", "", "")
		id := f.String("command-id", "", "")
		drive := f.Bool("drive", false, "")
		inputs := bindings{}
		f.Var(inputs, "input", "")
		refFiles := bindings{}
		f.Var(refFiles, "input-ref", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *workflow == "" || *brief == "" {
			return usageError("run start requires --workflow FILE and --brief FILE")
		}
		if *id == "" {
			*id = commandID()
		}
		refs := map[string]prifly.ArtifactRef{}
		for port, path := range refFiles {
			var ref prifly.ArtifactRef
			if err := readJSON(c.requestFile(path), &ref); err != nil {
				return err
			}
			refs[port] = ref
		}
		result, err := e.Start(ctx, prifly.StartOptions{CommandID: *id, WorkflowFile: *workflow, BriefFile: *brief, Inputs: inputs, InputRefs: refs})
		if err != nil {
			return err
		}
		if !*drive {
			return c.emit(commandResponse(result))
		}
		if err := e.Drive(ctx, result.Receipt.RunID); err != nil {
			return err
		}
		view, err := e.View(ctx, result.Receipt.RunID)
		if err != nil {
			return err
		}
		return c.emit(view)
	}
	if args[0] == "fork" {
		f := flags("run fork")
		file := f.String("file", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return usageError("run fork requires --file FILE")
		}
		var command prifly.ForkCommand
		if err := readJSON(c.requestFile(*file), &command); err != nil {
			return err
		}
		result, err := e.Fork(ctx, command)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	}
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		return usageError("An explicit run ID is required immediately after the run operation")
	}
	id := args[1]
	f := flags("run " + args[0])
	command := f.String("command-id", "", "")
	reason := f.String("reason", "", "")
	switch args[0] {
	case "decisions":
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		view, err := e.View(ctx, id)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "run-decision-ledger/1", "run_id": id, "run_version": view.RunVersion, "package_profile": decisionProfile(view.Run.DecisionSheet), "records": view.Run.DecisionLedger, "pending": view.Run.PendingDecision})
	case "decision":
		if len(args) < 3 {
			return usageError("run decision RUN_ID request --attempt ID --envelope-digest DIGEST --decision ID --expected-run-version N | answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON")
		}
		switch args[2] {
		case "request":
			attemptID := f.String("attempt", "", "current session attempt ID")
			envelopeDigest := f.String("envelope-digest", "", "current session envelope digest")
			decisionID := f.String("decision", "", "declared runtime decision ID")
			expectedVersion := f.Int64("expected-run-version", -1, "current Run version from session task")
			if err := parse(f, args[3:]); err != nil {
				return err
			}
			if *attemptID == "" || *envelopeDigest == "" || *decisionID == "" || *expectedVersion < 0 {
				return usageError("run decision RUN_ID request requires --attempt ID --envelope-digest DIGEST --decision ID --expected-run-version N")
			}
			view, err := e.View(ctx, id)
			if err != nil {
				return err
			}
			definitionDigest := "sha256:undeclared"
			if view.Run.DecisionCatalog != nil {
				for _, definition := range view.Run.DecisionCatalog.Decisions {
					if definition.ID != *decisionID {
						continue
					}
					definitionDigest, err = prifly.DecisionDefinitionDigest(definition)
					if err != nil {
						return err
					}
					break
				}
			}
			result, err := e.RequestDecision(ctx, prifly.DecisionRequest{SchemaVersion: prifly.DecisionRequestVersion, RunID: id, AttemptID: *attemptID, EnvelopeDigest: *envelopeDigest, DecisionID: *decisionID, DefinitionDigest: definitionDigest, ExpectedRunVersion: *expectedVersion})
			if err != nil {
				return err
			}
			return c.emit(commandResponse(result))
		case "answer":
			decisionID := f.String("decision", "", "pending decision ID")
			requestDigest := f.String("request-digest", "", "digest of the pending decision request")
			expectedVersion := f.Int64("expected-run-version", -1, "current Run version from run decisions")
			value := f.String("value", "", "typed JSON answer for the pending decision")
			if err := parse(f, args[3:]); err != nil {
				return err
			}
			if *decisionID == "" || *requestDigest == "" || *expectedVersion < 0 || *value == "" {
				return usageError("run decision RUN_ID answer requires --decision ID --request-digest DIGEST --expected-run-version N --value JSON")
			}
			view, err := e.View(ctx, id)
			if err != nil {
				return err
			}
			answer, err := flow.Canonical([]byte(*value))
			if err != nil {
				return usageError("run decision answer --value must be JSON")
			}
			request := view.Run.PendingDecision
			if request == nil || request.DecisionID != *decisionID || view.RunVersion != *expectedVersion {
				return usageError("run decision answer does not match the current pending decision")
			}
			digest, err := prifly.DecisionRequestDigest(*request)
			if err != nil || digest != *requestDigest {
				return usageError("run decision answer does not match the current pending decision")
			}
			result, err := e.AnswerDecision(ctx, prifly.DecisionAnswer{SchemaVersion: prifly.DecisionAnswerVersion, RunID: id, DecisionID: *decisionID, DefinitionDigest: request.DefinitionDigest, RequestDigest: *requestDigest, ExpectedRunVersion: *expectedVersion, Value: answer})
			if err != nil {
				return err
			}
			return c.emit(commandResponse(result))
		default:
			return usageError("run decision RUN_ID request --attempt ID --envelope-digest DIGEST --decision ID --expected-run-version N | answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON")
		}
	case "status", "timing", "next", "explain", "drive":
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if args[0] == "drive" {
			if err := e.Drive(ctx, id); err != nil {
				return err
			}
		}
		if args[0] == "next" || args[0] == "explain" {
			result, err := e.Next(ctx, id)
			if err != nil {
				return err
			}
			return c.emit(result)
		}
		view, err := e.View(ctx, id)
		if err != nil {
			return err
		}
		if args[0] == "timing" {
			if c.format == "text" {
				return renderTiming(c.out, view.Timing, view.Cut)
			}
			version := "foundation-timing-view/1"
			if view.Run.Profile == flow.CoreProfile {
				version = "core-timing-view/1"
			}
			return c.emit(map[string]any{"schema_version": version, "cut": view.Cut, "run_version": view.RunVersion, "timing": view.Timing})
		}
		return c.emit(view)
	case "events":
		after := f.Int64("after", 0, "")
		limit := f.Int("limit", 100, "")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		view, err := e.Events(ctx, id, *after, *limit)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-events/1", "view": view})
	case "pause", "cancel", "stop":
		kind := f.String("kind", "pause", "")
		invocation := f.String("invocation", "", "restrict this invocation and its descendants within the selected Run")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *reason == "" {
			return usageError("--reason is required for a restriction")
		}
		if args[0] != "stop" {
			*kind = args[0]
		}
		if *command == "" {
			*command = commandID()
		}
		scope, scopeID := "run", id
		if *invocation != "" {
			view, err := e.View(ctx, id)
			if err != nil {
				return err
			}
			if view.Run.Invocations[*invocation] == nil {
				return usageError("--invocation must belong to the explicitly selected Run")
			}
			scope, scopeID = "invocation", *invocation
		}
		result, err := e.Restrict(ctx, prifly.RestrictCommand{SchemaVersion: "1", CommandID: *command, Scope: scope, ScopeID: scopeID, Kind: *kind, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "release":
		epoch := f.Int64("expected-epoch", -1, "")
		stops := stringsFlag{}
		f.Var(&stops, "stop", "")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *epoch < 0 || len(stops) == 0 || *reason == "" {
			return usageError("release requires --expected-epoch N --stop ID:GENERATION --reason TEXT; it does not resume")
		}
		refs := []prifly.StopGeneration{}
		for _, s := range stops {
			idx := strings.LastIndex(s, ":")
			if idx < 1 {
				return usageError("--stop must contain ID:GENERATION")
			}
			generation, err := strconv.ParseInt(s[idx+1:], 10, 64)
			if err != nil || generation < 1 {
				return usageError("invalid stop generation")
			}
			refs = append(refs, prifly.StopGeneration{ID: s[:idx], Generation: generation})
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.Release(ctx, prifly.ReleaseRequest{CommandID: *command, RunID: id, ExpectedControlEpoch: *epoch, Stops: refs, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "waivers":
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		view, err := e.View(ctx, id)
		if err != nil {
			return err
		}
		return c.emit(prifly.WaiverView(view.Run))
	case "waive":
		step := f.String("step", "", "")
		checkID := f.String("check-id", "", "")
		checkVersion := f.String("check-version", "", "")
		checkDigest := f.String("check-digest", "", "")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *step == "" || *checkID == "" || *checkVersion == "" || *checkDigest == "" || *reason == "" {
			return usageError("run waive RUN_ID --step STEP --check-id ID --check-version X.Y.Z --check-digest DIGEST --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.Waive(ctx, prifly.WaiveRequest{CommandID: *command, RunID: id, StepID: *step,
			CheckRef: flow.Ref{ID: *checkID, Version: *checkVersion, Digest: *checkDigest}, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "resume":
		version := f.Int64("expected-version", -1, "")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *version < 0 || *reason == "" {
			return usageError("resume requires --expected-version N --reason TEXT; release active stops separately")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.Resume(ctx, id, *command, *reason, *version)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	default:
		return usageError("Unsupported run operation; automatic retry and terminal reopening are not available")
	}
}

// A grant bounds when a decision is made, never who may hold the object: it
// cannot deliver a right its subject does not already have.
func (c *cli) grant(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("grant requires issue|revoke|list")
	}
	f := flags("grant " + args[0])
	command := f.String("command-id", "", "")
	reason := f.String("reason", "", "")
	id := f.String("id", "", "")
	switch args[0] {
	case "list":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		control, _, err := e.Control(ctx)
		if err != nil {
			return err
		}
		return c.emit(e.ControlGrantView(control))
	case "issue":
		subject := f.String("subject", "", "")
		capabilities := stringsFlag{}
		f.Var(&capabilities, "capability", "control operation this grant delegates")
		resources := stringsFlag{}
		f.Var(&resources, "resource", "exact action resource scope JSON file (repeatable)")
		operations := f.Int64("max-operations", 0, "")
		lifetime := f.Int64("lifetime-ms", 0, "")
		approvals := stringsFlag{}
		f.Var(&approvals, "approval", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *subject == "" || len(capabilities) == 0 || *operations < 1 || *lifetime < 1 || *reason == "" {
			return usageError("grant issue requires --subject ID --capability OP --max-operations N --lifetime-ms MS --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		scopes := make([]prifly.ResourceIdentity, 0, len(resources))
		for _, path := range resources {
			var resource prifly.ResourceIdentity
			if err := readJSON(c.requestFile(path), &resource); err != nil {
				return err
			}
			scopes = append(scopes, resource)
		}
		result, err := e.IssueControlGrant(ctx, prifly.ControlGrantRequest{CommandID: *command, SubjectID: *subject, Capabilities: capabilities, ResourceScopes: scopes, MaxOperations: *operations, LifetimeMS: *lifetime, Approvals: approvals, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "revoke":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *reason == "" {
			return usageError("grant revoke requires --id GRANT --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.RevokeControlGrant(ctx, prifly.ControlGrantRevoke{CommandID: *command, GrantID: *id, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	}
	return usageError("grant requires issue|revoke|list")
}

// An approval is a decision about one exact protected payload, not a switch.
// Its quorum and independence are frozen when it is opened.
// capacity reads or changes how many attempts this authority admits at once.
// The number a workflow declares is a separate statement; the smaller governs.
func (c *cli) capacity(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("capacity requires show or set")
	}
	f := flags("capacity " + args[0])
	switch args[0] {
	case "show":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		capacity, held, err := e.AdmissionCapacity(ctx)
		if err != nil {
			return err
		}
		queue, err := e.AdmissionQueue(ctx)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "1", "capacity": capacity, "held": held, "waiting": queue})
	case "set":
		capacity := f.Int64("capacity", 0, "attempts admitted at once")
		reason := f.String("reason", "", "")
		command := f.String("command-id", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *capacity == 0 || *reason == "" {
			return usageError("capacity set requires --capacity N --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.SetAdmissionCapacity(ctx, prifly.CapacityRequest{CommandID: *command, Capacity: *capacity, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	}
	return usageError("capacity requires show or set")
}

func (c *cli) approval(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("approval requires policy|request|decide|revoke|list")
	}
	f := flags("approval " + args[0])
	command := f.String("command-id", "", "")
	reason := f.String("reason", "", "")
	id := f.String("id", "", "")
	switch args[0] {
	case "list":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		control, _, err := e.Control(ctx)
		if err != nil {
			return err
		}
		return c.emit(e.ControlApprovalView(control))
	case "policy":
		operations := stringsFlag{}
		f.Var(&operations, "operation", "control operation that requires approval")
		quorum := f.Int("quorum", 1, "")
		independence := f.String("independence", "different_from_proposer", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *reason == "" {
			return usageError("approval policy requires --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.SetControlApprovalPolicy(ctx, prifly.ControlApprovalPolicyRequest{CommandID: *command, Operations: operations, Quorum: *quorum, Independence: *independence, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "request":
		operation := f.String("operation", "", "")
		digest := f.String("intent-digest", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *operation == "" || *digest == "" || *reason == "" {
			return usageError("approval request requires --operation OP --intent-digest DIGEST --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.RequestControlApproval(ctx, prifly.ApprovalRequest{CommandID: *command, Operation: *operation, IntentDigest: *digest, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "decide":
		decision := f.String("decision", "", "approve or reject")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *decision == "" || *reason == "" {
			return usageError("approval decide requires --id APPROVAL --decision approve|reject --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.DecideControlApproval(ctx, prifly.ApprovalDecision{CommandID: *command, ApprovalID: *id, Decision: *decision, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "revoke":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *reason == "" {
			return usageError("approval revoke requires --id APPROVAL --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.RevokeControlApproval(ctx, prifly.ApprovalRevoke{CommandID: *command, ApprovalID: *id, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	}
	return usageError("approval requires policy|request|decide|revoke|list")
}

// The host is not a process this authority started, so it reads its task and
// reports through explicit commands rather than through a pipe we own.
func (c *cli) session(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("session requires task|publish|action|submit|disconnect")
	}
	f := flags("session " + args[0])
	run := f.String("run", "", "")
	switch args[0] {
	case "task":
		// A Run may hold several handoffs at once, so a host names the attempt
		// it came for. Without a name it gets the first, and --all shows every
		// outstanding one rather than implying there is only this one.
		attempt := f.String("attempt", "", "the outstanding attempt this host came for")
		all := f.Bool("all", false, "list every outstanding handoff")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *run == "" {
			return usageError("session task requires --run RUN_ID")
		}
		if *all {
			tasks, err := e.SessionTasks(ctx, *run)
			if err != nil {
				return err
			}
			return c.emit(map[string]any{"schema_version": "1", "run_id": *run, "tasks": tasks})
		}
		task, err := e.SessionTask(ctx, *run, *attempt)
		if err != nil {
			return err
		}
		return c.emit(task)
	case "publish":
		file := f.String("file", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return usageError("session publish requires --file COMMAND.json")
		}
		data, err := readFile(*file, prifly.MaxDefinitionBytes)
		if err != nil {
			return err
		}
		command, err := prifly.ParsePublishCommand(data)
		if err != nil {
			return usageError("Invalid closed publication request")
		}
		result, err := e.PublishSessionPublication(ctx, command)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "action":
		file := f.String("file", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return usageError("session action requires --file COMMAND.json")
		}
		data, err := readFile(*file, prifly.MaxDefinitionBytes)
		if err != nil {
			return err
		}
		command, err := prifly.ParseProposeActionCommand(data)
		if err != nil {
			return usageError("Invalid closed action proposal")
		}
		result, err := e.ProposeSessionAction(ctx, command)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "submit":
		file := f.String("file", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" {
			return usageError("session submit requires --file SUBMISSION.json")
		}
		var submission prifly.SessionSubmission
		if _, err := readJSONBytes(*file, &submission); err != nil {
			return err
		}
		result, err := e.SubmitSession(ctx, submission)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	case "disconnect":
		attempt := f.String("attempt", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *run == "" || *attempt == "" {
			return usageError("session disconnect requires --run RUN_ID --attempt ATTEMPT_ID")
		}
		result, err := e.MarkSessionDisconnected(ctx, *run, *attempt)
		if err != nil {
			return err
		}
		return c.emit(commandResponse(result))
	}
	return usageError("session requires task|publish|action|submit|disconnect")
}

// A claim is authority state, so a crashed session leaves an owned record
// instead of an anonymous directory nobody is allowed to remove.
func (c *cli) claims(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("claim requires create|create-set|list|heartbeat|release")
	}
	f := flags("claim " + args[0])
	command := f.String("command-id", "", "")
	switch args[0] {
	case "list":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		record, err := e.Claims(ctx)
		if err != nil {
			return err
		}
		return c.emit(prifly.ClaimView(record))
	case "create-set":
		repositories := stringsFlag{}
		f.Var(&repositories, "repository", "git repository to claim (repeat for an atomic set)")
		base := f.String("base", "HEAD", "")
		owner := f.String("owner", "", "run or session identity that owns the claims")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if len(repositories) == 0 || *owner == "" {
			return usageError("claim create-set requires --repository DIR (repeatable) --owner ID")
		}
		if *command == "" {
			*command = commandID()
		}
		requests := []prifly.ClaimRequest{}
		for _, repository := range repositories {
			requests = append(requests, prifly.ClaimRequest{Repository: repository, BaseRef: *base, OwnerID: *owner})
		}
		claims, err := e.ClaimWorktrees(ctx, *command, requests)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-claims/1", "claims": claims})
	case "create":
		repository := f.String("repository", "", "git repository to claim a worktree from")
		base := f.String("base", "HEAD", "")
		owner := f.String("owner", "", "run or session identity that owns the claim")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *repository == "" || *owner == "" {
			return usageError("claim create requires --repository DIR --owner ID")
		}
		if *command == "" {
			*command = commandID()
		}
		claim, err := e.ClaimWorktree(ctx, prifly.ClaimRequest{CommandID: *command, Repository: *repository, BaseRef: *base, OwnerID: *owner})
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-claim/1", "claim": claim})
	case "heartbeat":
		id := f.String("id", "", "")
		generation := f.Int64("generation", 0, "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *generation < 1 {
			return usageError("claim heartbeat requires --id CLAIM --generation N")
		}
		if *command == "" {
			*command = commandID()
		}
		claim, err := e.HeartbeatClaim(ctx, prifly.ClaimHeartbeatRequest{CommandID: *command, ClaimID: *id, Generation: *generation})
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-claim/1", "claim": claim})
	case "release":
		id := f.String("id", "", "")
		generation := f.Int64("generation", 0, "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *generation < 1 {
			return usageError("claim release requires --id CLAIM --generation N")
		}
		if *command == "" {
			*command = commandID()
		}
		claim, err := e.ReleaseWorktree(ctx, prifly.ClaimReleaseRequest{CommandID: *command, ClaimID: *id, Generation: *generation})
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-claim/1", "claim": claim})
	}
	return usageError("claim requires create|list|release")
}

// Import checks and registers; it never runs anything the package carries.
func (c *cli) packages(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("package requires import|list|inspect|verify|remove|quarantine|revoke|restore|trust-root")
	}
	f := flags("package " + args[0])
	switch args[0] {
	case "list":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		record, err := e.Packages(ctx)
		if err != nil {
			return err
		}
		return c.emit(prifly.PackageView(record))
	case "inspect":
		refFile := f.String("ref", "", "exact package ImmutableRef JSON file")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *refFile == "" {
			return usageError("package inspect requires --ref FILE with an ImmutableRef")
		}
		var ref flow.Ref
		if err := readJSON(c.requestFile(*refFile), &ref); err != nil {
			return err
		}
		data, _ := json.Marshal(ref)
		if err := flow.ValidateProtocol("ImmutableRef", data); err != nil {
			return err
		}
		inspection, err := e.InspectPackage(ctx, ref)
		if err != nil {
			return err
		}
		return c.emit(inspection)
	case "verify":
		id := f.String("id", "", "")
		version := f.String("version", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *version == "" {
			return usageError("package verify requires --id ID --version X.Y.Z")
		}
		report, err := e.VerifyPackage(ctx, *id, *version)
		if err != nil {
			return err
		}
		return c.emit(report)
	case "remove", "quarantine", "revoke", "restore":
		id := f.String("id", "", "")
		version := f.String("version", "", "")
		reason := f.String("reason", "", "")
		command := f.String("command-id", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *version == "" || *reason == "" {
			return usageError("package " + args[0] + " requires --id ID --version X.Y.Z --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		status := map[string]string{"remove": "removed", "quarantine": "quarantined", "revoke": "revoked", "restore": "trusted"}[args[0]]
		result, err := e.SetPackageStatus(ctx, prifly.PackageLifecycleRequest{CommandID: *command, ID: *id, Version: *version, Status: status, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "trust-root":
		id := f.String("id", "", "")
		key := f.String("public-key", "", "hex ed25519 public key")
		note := f.String("note", "", "")
		remove := f.Bool("remove", false, "")
		reason := f.String("reason", "", "")
		command := f.String("command-id", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *id == "" || *reason == "" || (!*remove && *key == "") {
			return usageError("package trust-root requires --id ID --public-key HEX --reason TEXT, or --id ID --remove --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.SetTrustRoot(ctx, prifly.TrustRootRequest{CommandID: *command, ID: *id, PublicKey: *key, Note: *note, Remove: *remove, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "import":
		dir := f.String("dir", "", "sealed package source directory")
		archive := f.String("archive", "", "sealed package archive")
		signature := f.String("signature", "", "detached signature over the manifest digest")
		reason := f.String("reason", "", "")
		command := f.String("command-id", "", "")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if (*dir == "") == (*archive == "") || *reason == "" {
			return usageError("package import requires exactly one of --dir DIR or --archive FILE, plus --reason TEXT")
		}
		if *command == "" {
			*command = commandID()
		}
		request := prifly.PackageImportRequest{CommandID: *command, Directory: *dir, Reason: *reason}
		var result local.AuthorityApplyResult
		var err error
		if *archive != "" {
			result, err = e.ImportPackageArchive(ctx, request, *archive, *signature)
		} else {
			result, err = e.ImportPackage(ctx, request)
		}
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	}
	return usageError("package requires import|list|inspect|verify|remove|quarantine|revoke|restore")
}

// Authority control is separate from a Run: an installation or project stop has
// no run id, and releasing it consumes a ControlIntent instead of inventing one.
func (c *cli) control(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("control requires status|stop|release")
	}
	f := flags("control " + args[0])
	scope := f.String("scope", "project", "installation or project")
	command := f.String("command-id", "", "")
	reason := f.String("reason", "", "")
	switch args[0] {
	case "status":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		control, version, err := e.Control(ctx)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-control/1", "control_version": version, "control": control})
	case "stop":
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *reason == "" {
			return usageError("--reason is required for a control stop")
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.RestrictControl(ctx, prifly.ControlRestrictRequest{CommandID: *command, Scope: *scope, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	case "release":
		epoch := f.Int64("expected-epoch", -1, "")
		stops := stringsFlag{}
		approvals := stringsFlag{}
		f.Var(&stops, "stop", "")
		f.Var(&approvals, "approval", "approval consumed by this release")
		grant := f.String("grant", "", "grant this release draws one operation from")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *epoch < 0 || len(stops) == 0 || *reason == "" {
			return usageError("control release requires --expected-epoch N --stop ID:GENERATION --reason TEXT")
		}
		refs := []prifly.StopGeneration{}
		for _, s := range stops {
			idx := strings.LastIndex(s, ":")
			if idx < 1 {
				return usageError("--stop must contain ID:GENERATION")
			}
			generation, err := strconv.ParseInt(s[idx+1:], 10, 64)
			if err != nil || generation < 1 {
				return usageError("invalid stop generation")
			}
			refs = append(refs, prifly.StopGeneration{ID: s[:idx], Generation: generation})
		}
		if *command == "" {
			*command = commandID()
		}
		result, err := e.ReleaseControl(ctx, prifly.ControlReleaseRequest{CommandID: *command, Scope: *scope, ExpectedControlEpoch: *epoch, Stops: refs, Approvals: approvals, GrantID: *grant, Reason: *reason})
		if err != nil {
			return err
		}
		return c.emit(authorityResponse(result))
	}
	return usageError("control requires status|stop|release")
}

func authorityResponse(result local.AuthorityApplyResult) map[string]any {
	return map[string]any{"schema_version": "foundation-control-command/1", "duplicate": result.Duplicate, "receipt": result.Receipt}
}

type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }
func commandResponse(result local.ApplyResult) map[string]any {
	return map[string]any{"schema_version": "foundation-command/1", "duplicate": result.Duplicate, "receipt": result.Receipt}
}

func readFile(path string, limit int64) ([]byte, error) {
	if path == "-" {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, limit+1))
		if int64(len(b)) > limit {
			return nil, local.ErrBlobLimit
		}
		return b, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, local.ErrUnsafePath
	}
	if st.Size() > limit {
		return nil, local.ErrBlobLimit
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, local.ErrBlobLimit
	}
	return b, err
}
func readJSON(path string, target any) error { _, err := readJSONBytes(path, target); return err }
func readJSONBytes(path string, target any) ([]byte, error) {
	b, err := readFile(path, prifly.MaxDefinitionBytes)
	if err != nil {
		return nil, err
	}
	b, err = flow.Canonical(b)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return nil, usageError("Invalid closed JSON request; check field names and value types")
	}
	return b, nil
}
func (c *cli) requestFile(path string) string {
	if path == "-" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.project, path)
}
func (c *cli) ref(args []string) error {
	if len(args) < 1 {
		return usageError("ref FILE --id ID --version SEMVER [--raw-text]")
	}
	path := args[0]
	f := flags("ref")
	id := f.String("id", "", "")
	version := f.String("version", "", "")
	rawText := f.Bool("raw-text", false, "hash exact UTF-8 resource bytes")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if *id == "" || *version == "" {
		return usageError("ref requires --id and --version")
	}
	b, err := readFile(c.requestFile(path), prifly.MaxDefinitionBytes)
	if err != nil {
		return err
	}
	var digest string
	if *rawText {
		if !utf8.Valid(b) {
			return &flow.Problem{Code: "invalid_unicode", Message: "Raw text resource must contain valid UTF-8 bytes."}
		}
		// This computes an identity only. Registry3 declares the resource's
		// encoding/media type explicitly; the flag grants no execution rights.
		digest = fmt.Sprintf("sha256:%x", sha256.Sum256(b))
	} else {
		format := "json"
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			format = "yaml"
		}
		var data []byte
		var err error
		if format == "yaml" {
			data, err = flow.WorkflowJSONBytes(b, format)
		} else {
			data, err = flow.JSONBytes(b, format)
		}
		if err != nil {
			return err
		}
		digest, err = flow.Digest(data)
		if err != nil {
			return err
		}
	}
	ref := flow.Ref{ID: *id, Version: *version, Digest: digest}
	data, _ := json.Marshal(ref)
	if err := flow.ValidateProtocol("ImmutableRef", data); err != nil {
		return err
	}
	return c.emit(ref)
}
func (c *cli) source(e *prifly.Engine, args []string) error {
	if len(args) == 0 || args[0] != "import" {
		return usageError("source import requires --file FILE and --type json|blob")
	}
	f := flags("source import")
	file := f.String("file", "", "explicit source file")
	format := f.String("type", "", "json or blob")
	schemaFile := f.String("schema-ref", "", "exact JSON schema reference file")
	media := f.String("media-type", "", "content media type")
	identity := f.String("external-identity", "", "declared external identity")
	version := f.String("external-version", "", "declared external version")
	scope := f.String("external-scope", "", "declared external scope, not acquisition permission")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if *file == "" || (*format != "json" && *format != "blob") {
		return usageError("source import requires --file FILE and --type json|blob")
	}
	var schema *flow.Ref
	if *schemaFile != "" {
		schema = &flow.Ref{}
		if err := readJSON(c.requestFile(*schemaFile), schema); err != nil {
			return err
		}
	}
	artifact, err := e.ImportSource(prifly.SourceImportOptions{
		Path: *file, Format: *format, SchemaRef: schema, MediaType: *media,
		ExternalIdentity: *identity, ExternalVersion: *version, ExternalScope: *scope,
	})
	if err != nil {
		return err
	}
	// The returned artifact describes the SourceSnapshot. Its content_ref
	// points to the separately sealed source bytes; neither is a live locator.
	return c.emit(map[string]any{"schema_version": "foundation-artifact/1", "ref": artifact.Ref(), "artifact": artifact})
}

func (c *cli) artifact(e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("artifact requires import|inspect|export")
	}
	f := flags("artifact " + args[0])
	file := f.String("file", "", "")
	refFile := f.String("ref", "", "")
	schemaFile := f.String("schema-ref", "", "")
	format := f.String("type", "blob", "")
	media := f.String("media-type", "", "")
	output := f.String("output", "", "")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if args[0] == "import" {
		if *file == "" {
			return usageError("artifact import requires --file")
		}
		var schema *flow.Ref
		if *schemaFile != "" {
			schema = &flow.Ref{}
			if err := readJSON(c.requestFile(*schemaFile), schema); err != nil {
				return err
			}
		}
		mediaArgs := []string{}
		if *media != "" {
			mediaArgs = append(mediaArgs, *media)
		}
		a, err := e.ImportArtifact(*file, *format, schema, mediaArgs...)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-artifact/1", "ref": a.Ref(), "artifact": a})
	}
	if *refFile == "" {
		return usageError("artifact inspect/export requires --ref FILE with an ArtifactRef")
	}
	var ref prifly.ArtifactRef
	if err := readJSON(c.requestFile(*refFile), &ref); err != nil {
		return err
	}
	switch args[0] {
	case "inspect":
		a, _, err := e.Artifact(ref)
		if err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-artifact/1", "ref": ref, "artifact": a})
	case "export":
		if *output == "" {
			return usageError("artifact export requires --output (existing files are never overwritten)")
		}
		if err := e.ExportArtifact(ref, *output); err != nil {
			return err
		}
		return c.emit(map[string]any{"schema_version": "foundation-export/1", "ref": ref, "output": *output, "exported": true})
	default:
		return usageError("Unsupported artifact operation")
	}
}
func (c *cli) telemetry(ctx context.Context, e *prifly.Engine, args []string) error {
	if len(args) == 0 {
		return usageError("telemetry query --file QUERY.json or telemetry catalog")
	}
	f := flags("telemetry")
	file := f.String("file", "", "")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	var q prifly.TelemetryQuery
	if args[0] == "catalog" {
		if err := json.Unmarshal([]byte(`{"schema_version":"telemetry-query/1","mode":"catalog"}`), &q); err != nil {
			return err
		}
	} else if args[0] == "query" && *file != "" {
		if err := readJSON(c.requestFile(*file), &q); err != nil {
			return err
		}
	} else {
		return usageError("telemetry query requires --file; compare/profiling/exporter backends are unsupported")
	}
	result, err := e.Telemetry(ctx, q)
	if err != nil {
		return err
	}
	return c.emit(result)
}
func (c *cli) step(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("step requires publish --file COMMAND.json or status")
	}
	f := flags("step")
	file := f.String("file", "", "")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	socket, token := os.Getenv("PRIFLY_SOCKET"), os.Getenv("PRIFLY_TOKEN")
	if socket == "" || token == "" {
		return usageError("Step commands require an admitted step's PRIFLY_SOCKET and PRIFLY_TOKEN")
	}
	method, path := http.MethodGet, "/status"
	var body io.Reader
	if args[0] == "publish" {
		if *file == "" {
			return usageError("step publish requires --file")
		}
		var cmd prifly.PublishCommand
		b, err := readJSONBytes(*file, &cmd)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
		method, path = http.MethodPost, "/publish"
	} else if args[0] != "status" {
		return usageError("Unsupported step operation; lifecycle is owned by the core")
	} else {
		values := url.Values{"run_id": {os.Getenv("PRIFLY_RUN_ID")}, "step_instance_id": {os.Getenv("PRIFLY_STEP_ID")}, "attempt_id": {os.Getenv("PRIFLY_ATTEMPT_ID")}, "envelope_digest": {os.Getenv("PRIFLY_ENVELOPE_DIGEST")}}
		path += "?" + values.Encode()
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, method, "http://prifly"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return errors.New("step_transport_unavailable: no receipt received; do not invent an acknowledgement")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("step_publication_rejected: inspect the publication contract and current attempt")
	}
	if !json.Valid(data) {
		return errors.New("invalid_transport_response: expected JSON receipt")
	}
	return c.emit(json.RawMessage(data))
}

func renderRun(w io.Writer, v prifly.RunView) error {
	outcome := "—"
	if v.Run.Outcome != nil {
		outcome = *v.Run.Outcome
	}
	if _, err := fmt.Fprintf(w, "Run %s\nstatus=%s outcome=%s version=%d cut=%d driver_live=%t\nprofile=%s trust=%s\n", strconv.Quote(v.Run.ID), v.Run.Status, outcome, v.RunVersion, v.Cut, v.DriverLive, v.Run.Profile, v.Run.TrustProfile); err != nil {
		return err
	}
	for _, stop := range v.Run.Stops {
		if stop.Status == "active" {
			if _, err := fmt.Fprintf(w, "active_stop=%s generation=%d kind=%s epoch=%d\n", strconv.Quote(stop.ID), stop.Generation, stop.Kind, v.Run.ControlEpoch); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(w, "diagnostics=%d outputs=%d unresolved=%t\n", len(v.Run.Diagnostics), len(v.Run.Outputs), v.Run.HasUnresolvedEffects); err != nil {
		return err
	}
	if v.Run.DecisionSheet != nil {
		if _, err := fmt.Fprintf(w, "decisions=%d pending_decision=%t\n", len(v.Run.DecisionLedger), v.Run.PendingDecision != nil); err != nil {
			return err
		}
	}
	return renderTiming(w, v.Timing, v.Cut)
}

func decisionProfile(sheet *prifly.DecisionSheet) string {
	if sheet == nil {
		return ""
	}
	return sheet.PackageProfile
}
func renderTiming(w io.Writer, t prifly.TimingTree, cut int64) error {
	if _, err := fmt.Fprintf(w, "Timing %s cut=%d as_of=%s\n", t.CalculatorRevision, cut, t.AsOf.UTC); err != nil {
		return err
	}
	var visit func(prifly.TimingNode, int) error
	visit = func(n prifly.TimingNode, depth int) error {
		duration := n.Metrics["elapsed"]
		value := "unknown"
		if duration.ValueMS != nil {
			value = strconv.FormatInt(*duration.ValueMS, 10) + "ms"
		} else if duration.KnownMS != nil {
			value = "known " + strconv.FormatInt(*duration.KnownMS, 10) + "ms"
		}
		if _, err := fmt.Fprintf(w, "%s%s %s status=%s elapsed=%s quality=%s\n", strings.Repeat("  ", depth), n.Kind, strconv.Quote(n.ID), n.Status, value, duration.Quality); err != nil {
			return err
		}
		for _, child := range n.Children {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(t.Root, 0)
}
func writeCSV(w io.Writer, value any) error {
	// Export the exact structured report as escaped JSON cells, not an Excel
	// formula. No numeric value/coverage is silently changed into a display zero.
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(b, &object); err != nil {
		return usageError("CSV requires a structured report")
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"field", "json_value"}); err != nil {
		return err
	}
	for _, key := range keys {
		if err := writer.Write([]string{csvSafe(key), csvSafe(string(object[key]))}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
func csvSafe(s string) string {
	trim := strings.TrimLeft(s, " \t\r\n")
	if len(trim) > 0 && strings.ContainsRune("=+-@", rune(trim[0])) || strings.HasPrefix(s, "\t") || strings.HasPrefix(s, "\r") || strings.HasPrefix(s, "\n") {
		return "'" + s
	}
	return s
}

const help = `Pri-Fly — deterministic local workflow runner

Global: --project DIR  --json  --format text|json|csv

  update                           Install the newest signed stable release for this managed binary
  init [--profile PROFILE] [DIR]    Empty installation; default foundation-sequence/1
  project init [--repository DIR] [--state-root DIR]
                                   Create a tracked .prifly project profile and separate local authority state
  project workflows [--repository DIR]
                                   List the project scenarios that the launcher may start and their declared inputs
  project workflows search [QUERY] [--category ID] [--catalog URL]
                                   Read a workflow catalog; discovery only, nothing is copied into the project
  project workflows add SOURCE [--ref REF] [--path DIR] [--name NAME] [--catalog URL] [--repository DIR]
                                   Copy one workflow folder from a Git repository or catalog entry into .prifly/workflows and declare it; nothing is sealed, trusted or executed
  project workflows update NAME [--ref REF] [--repository DIR]
                                   Refresh an installed folder to its tracked ref; local edits are refused and extend.yaml is kept
  project workflows remove NAME [--repository DIR]
                                   Delete the folder and its launches from the tracked profile; authority packages and Runs stay
  project questionnaire --repository DIR (--package NAME|--launch ID)
                                   Read declared launch decisions without creating a Run
  project runners update [--repository DIR]
                                   Replace only exact known generated host runners; customized files are refused
  project compile --repository DIR --package NAME --host codex-cli|codex-app|claude-code --output DIR [--value NAME=JSON]
                                   Seal one declared YAML package; import remains a separate owner decision
  project start --repository DIR --launch ID --host codex-cli|codex-app|claude-code --brief FILE [--input PORT=FILE] [--input-ref PORT=REF.json] [--workspace worktree|checkout]
                                   Seal, claim and drive one declared launch to its first honest handoff; direct CLI defaults to worktree
  capabilities                     Implemented contracts/profiles, not permission grants
  version | doctor | inventory     Versions, integrity, exact local definitions
  ref FILE --id ID --version X.Y.Z [--raw-text]
                                   Canonical JSON/YAML; --raw-text hashes exact UTF-8 resource bytes
  schema NAME                      Exact baseline, foundation or core JSON Schema
  validate --workflow FILE          Shape, refs, graph and profile validation
  preview --workflow FILE [--brief FILE] [--input-ref PORT=REF.json]
  run start --workflow FILE --brief FILE [--input PORT=FILE] [--input-ref PORT=REF.json] [--drive]

  run fork --file REQUEST.json      Create a linked Run from exact sealed refs; old Run is unchanged
  run status|next|explain|events|timing RUN_ID
  run drive RUN_ID                  Foreground owner; interrupt requests cancel
  run decisions RUN_ID              Read the sealed decision ledger and pending question
  run decision RUN_ID request --attempt ID --envelope-digest DIGEST --decision ID --expected-run-version N
                                   Compatible executor requests one declared runtime decision
  run decision RUN_ID answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON
                                   Answer exactly the current pending decision; stale answers are refused
  run pause|cancel RUN_ID --reason TEXT [--command-id ID]
  run release RUN_ID --expected-epoch N --stop ID:GENERATION --reason TEXT
  run resume RUN_ID --expected-version N --reason TEXT
  run waive RUN_ID --step STEP --check-id ID --check-version X.Y.Z --check-digest DIGEST --reason TEXT
  run waivers RUN_ID                A waiver is not a pass: the outcome stays completed_with_waivers
  control status                    Enrolled session principal, object access and control stops
  control stop --scope installation|project --reason TEXT [--command-id ID]
  control release --scope installation|project --expected-epoch N --stop ID:GENERATION --reason TEXT
                                   Authority-side stops; they forbid new admissions, not running work
  package import --dir DIR --reason TEXT [--command-id ID]
                                   Seal, verify and trust a local package; nothing in it is executed
  package list                      Trusted packages, their origin and recorded trust decision
  package inspect --ref REF.json    Read one exact sealed package; never executes or repairs it
  package import --archive FILE.tar [--signature FILE.sig] --reason TEXT
  package trust-root --id ID --public-key HEX --reason TEXT | --id ID --remove --reason TEXT
                                   A key inside a package never appoints itself trusted
  package verify --id ID --version X.Y.Z
                                   Re-read every sealed byte; a mismatch is reported, never repaired
  package remove|quarantine|revoke|restore --id ID --version X.Y.Z --reason TEXT
                                   Remove refuses while a run holds it; revoke also blocks old pins
  claim create --repository DIR --owner ID [--base REF]
  claim create-set --repository DIR --repository DIR --owner ID
                                   One decision for the whole set: all resources or none
  claim list | claim heartbeat --id CLAIM --generation N | claim release --id CLAIM --generation N
                                   An expired lease blocks a conflicting claim; it never hands ownership over
  session task --run RUN_ID         The sealed handoff an assisted host currently holds
  session publish --file COMMAND.json
                                   Publish a sealed artifact before the assisted attempt settles
  action propose --file COMMAND.json
                                   Seal one exact ActionIntent proposal; this never executes it
  action admit --file COMMAND.json
                                   Consume exact approvals and retain ActionAdmission; this never delivers it
  session action --file COMMAND.json
                                   Backward-compatible alias for action propose
  session submit --file SUBMISSION.json
                                   Report for exactly the attempt and envelope that were handed over
  session disconnect --run RUN_ID --attempt ATTEMPT_ID
                                   Record an expired handoff as unknown; it never becomes success
  approval policy --operation OP --quorum N --independence RULE --reason TEXT
  approval request --operation OP --intent-digest DIGEST --reason TEXT
  approval decide --id APPROVAL --decision approve|reject --reason TEXT
  approval revoke --id APPROVAL --reason TEXT | approval list
                                   One owner cannot form an independent quorum; such a decision is refused
  grant issue --subject ID --capability OP [--resource RESOURCE.json] --max-operations N --lifetime-ms MS --reason TEXT
  grant revoke --id GRANT --reason TEXT | grant list
                                   Bounded delegation; issuing it is gated exactly like what it delegates
  monitor [--addr 127.0.0.1:7777]   Read-only view of recorded runs in a browser; loopback only, offers no command
  capacity show | capacity set --capacity N --reason TEXT
                                   How many attempts run at once here; a workflow declares its own and the smaller governs
  command receipt --id COMMAND_ID   Inspect a lost response without re-running
  artifact import --file FILE [--type json|blob] [--schema-ref REF.json]
  artifact inspect --ref REF.json
  artifact export --ref REF.json --output FILE
  source import --file FILE --type json|blob [--schema-ref REF.json] [--media-type MIME]
    [--external-identity TEXT] [--external-version TEXT] [--external-scope TEXT]
                                   Returns a SourceSnapshot artifact; external metadata is unverified
  task prepare --input TASK.json  Validates TaskInput/1, seals it as a SourceSnapshot and writes a RunBrief
  step publish --file COMMAND.json | step status   (inside an admitted step)
  telemetry catalog | telemetry query --file QUERY.json

Release and resume are separate; neither silently starts a background driver.
Use --command-id to retry the same command. Exit 0 confirms a command/read,
not a successful workflow outcome. Read the typed status/outcome/verdict.
Local executables are trusted and run with your OS rights; this is not a sandbox.
Opt in with init --profile core-workflow/1 for on_error, JSON projections,
declared input defaults, choices, calls and bounded repeat.
Full context and automatic checks require core-configuration/2 and
core:adapter/local-process@2.0.0; automatic check executors use operation check.
Existing Runs retain their pinned semantics.
Not yet supported: external effect delivery/execution (ActionDelivery),
unattended execution, automatic retry/reconcile/compensation, generic external
subscriptions, backpressure/retention/GC, provider usage, comparisons and profiling.
`
