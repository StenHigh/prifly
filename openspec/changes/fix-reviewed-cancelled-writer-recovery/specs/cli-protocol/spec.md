## ADDED Requirements

### Requirement: Project CLI объясняет reviewed recovery после cancel и resolve

`project continue --source-run RUN --launch SAME --prepare` и `project recover --prepare` SHALL поддерживать failed source после отмены writer и inspected resolution по общим правилам recovery. Они SHALL показывать execute frontier, accepted prefix и existing claim с original inputs/decisions; start SHALL требовать актуальный review digest и повторять проверки. Source cancellation MUST NOT сниматься через `run resume`, повторный cancel или прямое изменение authority.

Документация CLI, authoring и runner SHALL отличать неизвестные effects, разрешённую obligation и отдельный конфликт native output. Пока effects неизвестны, precise refusal `recover_source_unsettled` SHALL требовать owner inspection/resolve. `applied` SHALL описываться как подтверждение effects, не завершённости результата. Занятый PLAN после takeover SHALL требовать owner сохранения bytes и следующего prepare от нового source, владеющего claim; другой workflow/host MUST NOT предлагаться как обход.

Current source set — `openspec/specs/cli-protocol/spec.md`; ownership и существующие CLI/review contracts сохраняются.

#### Scenario: Review остаётся read-only
- **WHEN** owner готовит допустимое recovery после resolution
- **THEN** source version/history, Run inventory, claim, HEAD/index и owner bytes не меняются

#### Scenario: Review устарел
- **WHEN** start использует digest до изменения source или target package
- **THEN** existing named stale refusal сохраняется до нового Run/takeover
