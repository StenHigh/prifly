## Context

`docs/spec/05-control-security-ux.md` owns 38 named control, security and UX
rules. `docs/operations.md` adds the executable F1 operating boundaries that
make those controls usable without claiming unsupported recovery. Both remain
the current source set until cutover.

## Goals / Non-Goals

**Goals:**

- Перенести named rules в descriptive OpenSpec requirements with testable
  scenarios.
- Сохранить every legacy acceptance link in the archived crosswalk and cover
  operating-guide boundaries that do not have old requirement IDs.
- Switch one source-map row only after source, crosswalk and replacement are
  reviewed.

**Non-Goals:**

- Не менять Go code, CLI behavior, YAML, JSON Schema, release evidence,
  manifests или historical operating files.
- Не обещать, что документальная проверка квалифицирует security profile либо
  закрывает product gate.

## Decisions

### Одна named boundary на каждое legacy правило

Permanent spec contains a descriptive requirement for each control, security
and UX rule, plus focused requirements for F1 operation boundaries. Grouping
them into a few generic security paragraphs would hide approval, uncertainty
and accessibility obligations during review.

### Legacy identifiers live only in the archived crosswalk

The archived matrix will hold the former control, security and UX record names,
their exact acceptance links and each replacement heading. The permanent spec
uses descriptive headings only, following the explicit owner decision that
OpenSpec is not a new custom ID authoring system.

### Operating guide is covered by named behavior, not copied command-by-command

The crosswalk records operating sections as source evidence. The permanent
spec keeps observable safety/operation contract; exact shell examples are
historical guidance until the final documentation cleanup decides their
replacement presentation.

### Cutover stays reversible until final cleanup

Apply validates all named records, their acceptance links and operating-source
coverage before changing only the source-map row. Legacy bytes stay untouched;
rollback is one map-row restore.

### Legacy coverage crosswalk

| Legacy source | Legacy record or section | Acceptance cases | Replacement requirement | Review |
|---|---|---|---|---|
| `docs/spec/05-control-security-ux.md` | `CTRL-001` | `AC-002` | «Core управляет маршрутом, а не текстом модели» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-002` | `AC-004`, `AC-024`, `AC-070`, `AC-107` | «Pinned contract пересекается с current restrictions» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-003` | `AC-072`, `AC-073`, `AC-114` | «Переход принимает только проверяемый result» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-004` | `AC-006`, `AC-095` | «Роли не имитируют независимость решений» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-005` | `AC-062`, `AC-079`, `AC-096`, `AC-099`, `AC-134` | «Каждый защищённый effect имеет exact intent» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-006` | `AC-095`, `AC-097`, `AC-098`, `AC-134` | «Approval имеет проверяемый lifecycle» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-007` | `AC-023`, `AC-064`, `AC-104` | «Grant делегирует только конечные права» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-008` | `AC-036`, `AC-105`, `AC-106` | «Неотменяемые checks остаются неотменяемыми» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-009` | `AC-080`, `AC-084` | «Retry не является тихим fallback» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-010` | `AC-099`, `AC-101`, `AC-102`, `AC-103` | «Stop имеет durable и исполнимую границу» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-011` | `AC-091`, `AC-100` | «Resume не обходит uncertainty или stop» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-012` | `AC-122` | «Limits различают measured, reserved и estimated» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-013` | `AC-007`, `AC-133` | «Оператор видит реального владельца выполнения» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-001` | `AC-109` | «Threat profile называет enforcement и остаточный риск» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-002` | `AC-074`, `AC-094` | «Access проверяется для каждого объекта и канала» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-003` | `AC-093` | «Identity не требует облачного аккаунта» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-004` | `AC-023`, `AC-062`, `AC-107` | «Host permission сильнее package и prompt» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-005` | `AC-061`, `AC-108` | «Tool calls и network destinations typed» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-006` | `AC-021`, `AC-027` | «Package trust ограничивает executable content» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-007` | `AC-008`, `AC-018`, `AC-092`, `AC-109` | «Sandbox обещает только qualified isolation» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-008` | `AC-059`, `AC-060` | «ContextManifest отделяет instructions от data» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-009` | `AC-066`, `AC-110` | «Adapter выдаёт scoped credentials» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-010` | `AC-066`, `AC-112`, `AC-113` | «Data policy контролирует каждый output channel» | verified |
| `docs/spec/05-control-security-ux.md` | `SEC-011` | `AC-094`, `AC-111` | «Protected evidence и callback имеют authenticated provenance» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-014` | `AC-014`, `AC-055`, `AC-068`, `AC-111` | «Evidence называет предмет и предел доказательства» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-015` | `AC-067`, `AC-069`, `AC-076` | «Verification использует первичные записи» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-016` | `AC-116`, `AC-120`, `AC-129` | «Authority имеет единую историю control facts» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-017` | `AC-039`, `AC-138` | «Explain и replay остаются read-only» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-018` | `AC-087`, `AC-103`, `AC-114` | «Incident control не создаёт force success» | verified |
| `docs/spec/05-control-security-ux.md` | `CTRL-019` | `AC-044`, `AC-088`, `AC-106` | «Final outcome отражает достигнутую работу» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-001` | `AC-131` | «CLI, API и panel используют одни понятия» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-002` | `AC-003`, `AC-132` | «Preview проверяет понимание до expensive effect» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-003` | `AC-133` | «Progress показывает известное, ожидаемое и unknown» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-004` | `AC-106`, `AC-134` | «Human decisions display exact consequences» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-005` | `AC-135` | «Error offers only safe next action» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-006` | `AC-136` | «Управление доступно без цветовой или pointer-only зависимости» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-007` | `AC-139` | «End-to-end profiles не навязывают coding workflow» | verified |
| `docs/spec/05-control-security-ux.md` | `UX-008` | `AC-140` | «Control mechanisms require qualification evidence» | verified |
| `docs/operations.md#1-установка` | Operating section 1 | — | «Local installation is explicit and non-destructive» | verified |
| `docs/operations.md#2-свой-yaml-workflow-и-конфигурация` | Operating section 2 | — | «Project profile separates versioned packages from local authority»; «Package compilation and executor protocol are explicit» | verified |
| `docs/operations.md#4-цель-и-закреплённые-входы` | Operating section 4 | — | «Pinned contract пересекается с current restrictions»; «Preview проверяет понимание до expensive effect» | verified |
| `docs/operations.md#5-управление-и-потерянный-ответ` | Operating section 5 | — | «Stop имеет durable и исполнимую границу»; «Resume не обходит uncertainty или stop» | verified |
| `docs/operations.md#6-hooks-и-телеметрия` | Operating section 6 | — | «Operational limits and telemetry remain bounded and truthful» | verified |
| `docs/operations.md#7-сбой-corruption-и-восстановление` | Operating section 7 | — | «Local failure handling preserves uncertainty» | verified |
| `docs/operations.md#8-пределы-и-удаление` | Operating section 8 | — | «Operational limits and telemetry remain bounded and truthful»; «Local failure handling preserves uncertainty» | verified |

## Risks / Trade-offs

- [A security rule is diluted during paraphrase] → review each named source
  record and preserve one requirement/scenario per boundary.
- [Operations commands become a second normative API] → retain only behavior
  contract in permanent spec and leave exact legacy examples unchanged until
  final cleanup.
- [Documentation validation is mistaken for qualification] → keep existing
  product gates and evidence untouched.

## Migration Plan

1. Build an archived matrix for all named records, their exact acceptance links
   and the operating-guide sections.
2. Verify each row against the replacement requirement and strict OpenSpec
   validation, then switch the source-map row.
3. Verify code/schema/evidence/manifest and legacy source preservation, archive
   the change, and run strict archived validation.
