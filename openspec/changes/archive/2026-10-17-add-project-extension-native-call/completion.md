# Завершение Core #9

2026-10-17: **25/25 задач выполнено** в явно согласованном scope Core 0.13.78.
`openspec instructions apply --change add-project-extension-native-call --json`
подтвердил `all_done`, total 25, complete 25, remaining 0 перед архивом.

Последовательные срезы evidence сохранены отдельно: инженерная реализация,
[release 0.13.78](release-0.13.78.md), затем
[live success/control](live-host-evidence.md) и [handoff W6](handoff-w6.md).
Ранние pending notes в первых двух документах отражают состояние на момент их
записи. Они сохранены byte-identical; актуальное завершение описано здесь.

Synthetic generated-runner matrix трёх hosts PASS. Реальные Codex app и Codex CLI
прошли success/control, native Runs settled. Live Claude Code **не проверен**:
владелец исключил его из приёмки 0.13.78 из-за отсутствия подписки/авторизации,
проверка записана в отдельный pending follow-up. Все native identities,
accepted refs/digests и trace hashes закреплены в `live-host-results.json`.

Перед архивом синхронизированы три main source sets: `workflow-and-context`
(1 MODIFIED, 5 ADDED), `cli-protocol` (2 ADDED), `specification-governance`
(1 ADDED). Повторное сравнение каждого delta подтвердило отсутствие неприменённых
изменений. Существующие scenarios заменённой requirement сохранены; остальные
147 requirements не изменены. Source ownership и public saved contracts прежние.
Строгая проверка всех specs/active changes: **34/34 PASS** перед перемещением.

Baseline hashes: 1482 protected historical/schema/workflow files unchanged;
две изменённые current authoring manifests относятся к уже выпущенной реализации.
Все 40 legacy fixture/test hashes unchanged. Исторические release evidence,
прежние archives и frozen contracts не переписаны.

W6 получает готовый handoff для независимой consumer qualification; его repository
прочитан без изменений. Приёмка browser/product policy, W6-A gates и P2 milestone
не объявлены завершёнными. Operational recovery исходных пользовательских Runs
по #7/#8 не выполнялся. Тег и binaries 0.13.78 остаются на прежнем release SHA.
