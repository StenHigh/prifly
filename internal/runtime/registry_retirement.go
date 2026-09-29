package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// RetiredEdition is an older trusted edition a launch withdraws from the
// registry to make room: its status becomes removed, nothing is deleted, and
// package restore undoes it.
type RetiredEdition struct {
	Ref     flow.Ref `json:"ref"`
	Entries int      `json:"entries"`
	Reason  string   `json:"reason"`
	// Undo is the command that trusts the edition again. A model asked how
	// to undo a withdrawal answered "launch the kept edition": the command
	// was only in the refusal it never saw.
	Undo string `json:"undo"`
}

// ProtectedEdition is an edition of the same package the plan may not
// withdraw, and why.
type ProtectedEdition struct {
	Ref    flow.Ref `json:"ref"`
	Reason string   `json:"reason"`
}

// RegistryRetirement is the plan to fit an edition into the registry by
// withdrawing older editions of the same package. Budget is the registry as
// the edition would leave it, After as the plan leaves it. PackagesVersion and
// Runs are what the plan was decided against: applying it is refused if
// either moved, so it never withdraws an edition a Run started on meanwhile.
type RegistryRetirement struct {
	Edition         flow.Ref           `json:"edition"`
	Budget          RegistryBudget     `json:"budget"`
	After           RegistryBudget     `json:"after"`
	Retire          []RetiredEdition   `json:"retire"`
	Protected       []ProtectedEdition `json:"protected,omitempty"`
	PackagesVersion int64              `json:"packages_version"`
	Runs            int64              `json:"runs"`
}

// PlanRegistryRetirement answers how an edition with extra definitions fits
// the registry. Nil when it fits as things are. Otherwise the fewest older
// trusted editions of the same package, oldest first by when they were
// imported, that no Run still in progress holds, no other package depends on,
// and that are not the newest earlier edition -- kept so going back is one
// launch away. When those are not enough, the refusal is dependency_limit and
// names what it could not withdraw and why. Nothing is written.
//
// Every build of a package is a new edition with its own definitions, so a
// project that follows its package filled the registry every few launches and
// a person had to withdraw old editions by hand; a host treating that as a
// deletion would not do it without its owner.
func (e *Engine) PlanRegistryRetirement(ctx context.Context, edition flow.Ref, extra int) (*RegistryRetirement, error) {
	file, err := e.localRegistry()
	if err != nil {
		return nil, err
	}
	record, version, err := e.readPackages(ctx)
	if err != nil {
		return nil, err
	}
	resolvable := resolvablePackages(record)
	packaged := extra
	for _, pkg := range record.Packages {
		if resolvable[pkg.Ref] {
			packaged += len(pkg.Components)
		}
	}
	total := len(file.Entries) + packaged
	budget := registryBudget(total)
	if budget.WouldRefuse == "" {
		return nil, nil
	}
	holders, runs, err := e.packageHoldersByComponent(ctx)
	if err != nil {
		return nil, err
	}
	older := []PackageEntry{}
	for _, pkg := range record.Packages {
		if pkg.Ref.ID == edition.ID && pkg.Ref.Version != edition.Version && resolvable[pkg.Ref] {
			older = append(older, pkg)
		}
	}
	imported := func(pkg PackageEntry) time.Time {
		at, _ := time.Parse(time.RFC3339Nano, pkg.Imported.UTC)
		return at
	}
	slices.SortFunc(older, func(a, b PackageEntry) int {
		if order := imported(a).Compare(imported(b)); order != 0 {
			return order
		}
		return strings.Compare(a.Ref.String(), b.Ref.String())
	})
	plan := &RegistryRetirement{Edition: edition, Budget: budget, Retire: []RetiredEdition{}, PackagesVersion: version, Runs: runs}
	for index, pkg := range older {
		reason := ""
		switch {
		case index == len(older)-1:
			reason = "the newest earlier edition is kept, so going back is one launch away"
		case len(dependents(record, pkg.Ref)) != 0:
			reason = "package " + dependents(record, pkg.Ref)[0] + " depends on it"
		default:
			for _, component := range pkg.Components {
				if held := holders[component.Ref]; len(held) != 0 {
					reason = "Run " + held[0] + " is still in progress on it"
					break
				}
			}
		}
		if reason != "" {
			plan.Protected = append(plan.Protected, ProtectedEdition{Ref: pkg.Ref, Reason: reason})
			continue
		}
		if total <= MaxLocalRegistryEntries {
			continue
		}
		plan.Retire = append(plan.Retire, RetiredEdition{Ref: pkg.Ref, Entries: len(pkg.Components), Reason: "oldest trusted edition of " + pkg.Ref.ID + " no Run holds; withdrawing deletes nothing", Undo: "package restore --id " + pkg.Ref.ID + " --version " + pkg.Ref.Version + " --reason TEXT"})
		total -= len(pkg.Components)
	}
	plan.After = registryBudget(total)
	if plan.After.WouldRefuse != "" {
		protected := make([]string, 0, len(plan.Protected))
		for _, kept := range plan.Protected {
			protected = append(protected, kept.Ref.Version+" ("+kept.Reason+")")
		}
		detail := "no older edition of " + edition.ID + " can be withdrawn"
		if len(protected) != 0 {
			detail = "the older editions of " + edition.ID + " that stay are " + strings.Join(protected, "; ")
		}
		return nil, fault("dependency_limit", fmt.Sprintf("edition %s needs %d registry entries and at most %d fit even after withdrawing %d older editions: %s; package list shows every trusted edition, and package remove --id ID --version VERSION --reason TEXT withdraws one you no longer run -- it deletes nothing and is undone by package restore --id ID --version VERSION --reason TEXT", edition.Version, budget.Entries, MaxLocalRegistryEntries, len(plan.Retire), detail))
	}
	return plan, nil
}

// packageHoldersByComponent names, for every definition a Run still in
// progress sealed, the Runs holding it, and counts every Run. The count is
// what a retirement is applied against.
func (e *Engine) packageHoldersByComponent(ctx context.Context) (map[flow.Ref][]string, int64, error) {
	snapshots, _, err := e.Store.ReadAllAt(ctx, -1, maxPinScanRuns)
	if err != nil {
		return nil, 0, err
	}
	if len(snapshots) >= maxPinScanRuns {
		return nil, 0, fault("scan_limit", "this installation holds more runs than one bounded scan can prove")
	}
	holders := map[flow.Ref][]string{}
	for _, snapshot := range snapshots {
		var r Run
		if err := decodeState(snapshot.Data, &r); err != nil {
			// A Run this build cannot read may hold anything: hold nothing
			// back on a guess, refuse the plan instead.
			return nil, 0, err
		}
		if r.terminal() {
			continue
		}
		for _, ref := range runPackageRefs(r) {
			holders[ref] = append(holders[ref], r.ID)
		}
	}
	return holders, int64(len(snapshots)), nil
}

// retireEditions applies a plan to the package record inside the authority
// transaction that trusts the edition it makes room for. The transaction is
// pinned to the record version and the Run count the plan was decided on, so
// the only check left here is that each edition is still what the plan saw.
func retireEditions(record *PackageRecord, plan *RegistryRetirement, obs Observation) error {
	for _, retired := range plan.Retire {
		found := false
		for index := range record.Packages {
			pkg := &record.Packages[index]
			if pkg.Ref != retired.Ref {
				continue
			}
			if pkg.withdrawn() || len(dependents(*record, pkg.Ref)) != 0 {
				return local.Reject("registry_plan_stale", "edition "+pkg.Ref.Version+" of "+pkg.Ref.ID+" changed after the plan; prepare the launch again")
			}
			pkg.Status = PackageRemoved
			pkg.StatusReason = "withdrawn to fit the registry for " + plan.Edition.ID + "@" + plan.Edition.Version + "; package restore --id " + pkg.Ref.ID + " --version " + pkg.Ref.Version + " --reason TEXT undoes it"
			observed := obs
			pkg.StatusChanged = &observed
			found = true
		}
		if !found {
			return local.Reject("registry_plan_stale", "edition "+retired.Ref.Version+" of "+retired.Ref.ID+" is no longer installed; prepare the launch again")
		}
	}
	return nil
}

// releaseInventoryPins frees the identities of withdrawn components, as
// package remove does: a Run carries its own sealed definitions, so nothing
// that ran is affected.
func (e *Engine) releaseInventoryPins(components []flow.Ref) error {
	for _, ref := range components {
		name := fmt.Sprintf("%x.json", sha256.Sum256([]byte(ref.ID+"@"+ref.Version)))
		if err := removeLocal(e.Root, filepath.Join(".prifly/inventory", name)); err != nil {
			return err
		}
	}
	return nil
}

// retiredComponents lists the components of the editions a plan withdraws.
func retiredComponents(record PackageRecord, plan *RegistryRetirement) []flow.Ref {
	components := []flow.Ref{}
	for _, retired := range plan.Retire {
		for _, pkg := range record.Packages {
			if pkg.Ref == retired.Ref {
				for _, component := range pkg.Components {
					components = append(components, component.Ref)
				}
			}
		}
	}
	return components
}
