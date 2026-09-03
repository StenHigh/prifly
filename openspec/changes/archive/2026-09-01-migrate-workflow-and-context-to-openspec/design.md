## Context

`docs/spec/03-packages-workflows-context.md` defines 15 package, 23 workflow
and 13 context boundaries. `docs/workflow-yaml.md` defines the compatible YAML
authoring/frontend and project-source path. Their implementation status and
acceptance links remain in `docs/roadmap/requirements-map.csv` and the chapter's
acceptance table. This is the current source set until cutover.

## Goals / Non-Goals

**Goals:**

- Move every legacy boundary into readable requirements and scenarios without
  the former internal identifiers in the permanent spec.
- Preserve a reviewable coverage mapping and acceptance meaning in the archived
  change, while making YAML an authoring frontend rather than a second runtime.

**Non-Goals:**

- Do not change Go runtime, YAML syntax, schema, JSON wire contracts, package
  compiler, release evidence or roadmap status.
- Do not delete the legacy sources, CSV maps, examples or historical manifests.

## Decisions

### One capability owns packages, graph, context and YAML authoring

The legacy source set describes one contract chain: declared source compiles to
locked definitions, definitions create bounded graph work, and attempts receive
bounded context. Splitting it into independent specs would create cross-capability
duplicates of refs, bindings, locking and authority. `workflow-and-context`
therefore owns this chain; runtime lifecycle, generic evidence and task intake
remain in their already separated capabilities.

### Legacy traceability remains archive-only

The completed archive MUST retain the source records and acceptance links below;
the permanent spec uses descriptive requirement names. This follows the owner's
choice not to preserve custom IDs as the live OpenSpec API.

### Legacy coverage crosswalk

| Legacy source records | Acceptance source | Replacement requirement set | Review |
|---|---|---|---|
| `PKG-001` … `PKG-015` | `requirements-map.csv`, `PKG-AC-01` … `PKG-AC-07` | package identity, inventory, locking, trust, configuration, adapter, diagnostics and YAML authoring requirements | verified |
| `WF-001` … `WF-008` | `requirements-map.csv`, `WF-AC-01`, `WF-AC-02`, `WF-AC-15` | workflow identity, deterministic controller, definitions, ports/bindings and no-source operation | verified |
| `WF-009` … `WF-015` | `requirements-map.csv`, `WF-AC-03` … `WF-AC-11` | closed choice, parallel/map/join, repeat/call and global budget requirements | verified |
| `WF-016` … `WF-023` | `requirements-map.csv`, `WF-AC-12` … `WF-AC-15` | wait, effects/retry, compensation, snapshots, admission, finish, preview and map requirements | verified |
| `CTX-001` … `CTX-013` | `requirements-map.csv`, `CTX-AC-01` … `CTX-AC-08` | manifest, trust separation, renderer, mechanical adapters, authority, bytes, dynamic context, limits, secrets, content checks and human change requirements | verified |
| `docs/workflow-yaml.md` | YAML authoring references and examples | lossless YAML, safe defaults, declared project source and compact workflow folder requirements | verified |

### Individual legacy record matrix

Each `requirements-map.csv#…` entry is the exact archived acceptance-link
record for the source requirement. The following matrix is deliberately kept
only in this migration archive; its replacement names have no legacy IDs.

| Legacy record | Acceptance-link record | Replacement requirement |
|---|---|---|
| `PKG-001` | `requirements-map.csv#PKG-001` | «Пустая установка остаётся рабочим состоянием» |
| `PKG-002` | `requirements-map.csv#PKG-002` | «Ответственность частей package разделена» |
| `PKG-003` | `requirements-map.csv#PKG-003` | «Executable reference имеет exact identity» |
| `PKG-004` | `requirements-map.csv#PKG-004` | «Package inventory проверяет exact bytes» |
| `PKG-005` | `requirements-map.csv#PKG-005` | «Run lock-ит всё исполняемое closure» |
| `PKG-006` | `requirements-map.csv#PKG-006` | «Установка отделена от исполнения» |
| `PKG-007` | `requirements-map.csv#PKG-007` | «Доверие package имеет независимое основание» |
| `PKG-008` | `requirements-map.csv#PKG-008` | «Requested capability не является разрешением» |
| `PKG-009` | `requirements-map.csv#PKG-009` | «Package обновляется без смены pinned history» |
| `PKG-010` | `requirements-map.csv#PKG-010` | «Uninstall и revocation сохраняют историю» |
| `PKG-011` | `requirements-map.csv#PKG-011` | «Пользователь может создать минимальный local step» |
| `PKG-012` | `requirements-map.csv#PKG-012` | «Configuration меняет values, а не поведение» |
| `PKG-013` | `requirements-map.csv#PKG-013` | «Adapter объявляет проверяемые свойства» |
| `PKG-014` | `requirements-map.csv#PKG-014` | «Интеграции остаются optional consumers» |
| `PKG-015` | `requirements-map.csv#PKG-015` | «Package показывает проверяемую пригодность» |
| `WF-001` | `requirements-map.csv#WF-001` | «Workflow отделяет definition, instances и operations» |
| `WF-002` | `requirements-map.csv#WF-002` | «Workflow отделяет definition, instances и operations» |
| `WF-003` | `requirements-map.csv#WF-003` | «Workflow отделяет definition, instances и operations» |
| `WF-004` | `requirements-map.csv#WF-004` | «Ports и bindings сохраняют exact provenance» |
| `WF-005` | `requirements-map.csv#WF-005` | «Ports и bindings сохраняют exact provenance» |
| `WF-006` | `requirements-map.csv#WF-006` | «Ports и bindings сохраняют exact provenance» |
| `WF-007` | `requirements-map.csv#WF-007` | «Workflow отделяет definition, instances и operations» |
| `WF-008` | `requirements-map.csv#WF-008` | «Workflow отделяет definition, instances и operations» |
| `WF-009` | `requirements-map.csv#WF-009` | «Workflow composition имеет closed control semantics» |
| `WF-010` | `requirements-map.csv#WF-010` | «Workflow composition имеет closed control semantics» |
| `WF-011` | `requirements-map.csv#WF-011` | «Workflow composition имеет closed control semantics» |
| `WF-012` | `requirements-map.csv#WF-012` | «Workflow composition имеет closed control semantics» |
| `WF-013` | `requirements-map.csv#WF-013` | «Repeat, call, wait и compensation остаются bounded» |
| `WF-014` | `requirements-map.csv#WF-014` | «Repeat, call, wait и compensation остаются bounded» |
| `WF-015` | `requirements-map.csv#WF-015` | «Все composition limits учитываются целиком» |
| `WF-016` | `requirements-map.csv#WF-016` | «Repeat, call, wait и compensation остаются bounded» |
| `WF-017` | `requirements-map.csv#WF-017` | «Repeat, call, wait и compensation остаются bounded» |
| `WF-018` | `requirements-map.csv#WF-018` | «Repeat, call, wait и compensation остаются bounded» |
| `WF-019` | `requirements-map.csv#WF-019` | «Workflow отделяет definition, instances и operations» |
| `WF-020` | `requirements-map.csv#WF-020` | «Все composition limits учитываются целиком» |
| `WF-021` | `requirements-map.csv#WF-021` | «Workflow terminal state и evolution имеют explicit contract» |
| `WF-022` | `requirements-map.csv#WF-022` | «Workflow terminal state и evolution имеют explicit contract» |
| `WF-023` | `requirements-map.csv#WF-023` | «Workflow composition имеет closed control semantics» |
| `CTX-001` | `requirements-map.csv#CTX-001` | «Context manifest ограничивает exact reading» |
| `CTX-002` | `requirements-map.csv#CTX-002` | «Context manifest ограничивает exact reading» |
| `CTX-003` | `requirements-map.csv#CTX-003` | «Context manifest ограничивает exact reading» |
| `CTX-004` | `requirements-map.csv#CTX-004` | «Context manifest ограничивает exact reading» |
| `CTX-005` | `requirements-map.csv#CTX-005` | «Mechanical execution и context never grant authority» |
| `CTX-006` | `requirements-map.csv#CTX-006` | «Mechanical execution и context never grant authority» |
| `CTX-007` | `requirements-map.csv#CTX-007` | «Mechanical execution и context never grant authority» |
| `CTX-008` | `requirements-map.csv#CTX-008` | «Dynamic context, limits и secrets остаются explicit» |
| `CTX-009` | `requirements-map.csv#CTX-009` | «Dynamic context, limits и secrets остаются explicit» |
| `CTX-010` | `requirements-map.csv#CTX-010` | «Dynamic context, limits и secrets остаются explicit» |
| `CTX-011` | `requirements-map.csv#CTX-011` | «Accepted result требует independent content checks» |
| `CTX-012` | `requirements-map.csv#CTX-012` | «Accepted result требует independent content checks» |
| `CTX-013` | `requirements-map.csv#CTX-013` | «Human absence и changed intent не угадываются» |
| `workflow-yaml.md#минимальная-форма` | YAML source itself | «YAML является lossless authoring frontend» |
| `workflow-yaml.md#компактный-yaml-шага` | YAML source itself | «YAML defaults ограничены безопасной структурой» |
| `workflow-yaml.md#project-package-source` | YAML source itself | «Project source компилируется из declared files» |
| `workflow-yaml.md#компактная-папка-сценария` | YAML source itself | «Compact workflow folder остаётся одним графом» |

### Cutover after source and contract review

Apply expands this crosswalk into individually reviewed source records before
marking it verified, checks every map link, syncs the permanent spec and changes
only the matching source-of-truth row. Legacy bytes remain unchanged; rollback
before final cleanup is a one-row source-map reversal.

## Risks / Trade-offs

- [A broad requirement hides a legacy edge case] → apply expands every range to
  an individual review checklist before cutover.
- [YAML appears to be a second runtime semantics] → requirements retain the
  mandatory lowering to canonical JSON before validation and Run.
- [Document validation is mistaken for release qualification] → tasks preserve
  runtime/evidence boundaries and report them separately.

## Migration Plan

1. Expand and verify the 51 numbered records, YAML contract sections and their
   central/chapter acceptance links against the candidate spec.
2. Sync the reviewed capability to `openspec/specs/workflow-and-context`, run
   strict validation and switch its source-map row without touching legacy bytes.
3. Verify code, schemas, evidence and manifests remain unchanged; archive the
   change with its completed crosswalk.
