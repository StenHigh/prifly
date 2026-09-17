# Evidence — let-a-clone-join-an-existing-project

Ворота на финальном дереве, 2026-09-18 (Apple M1, go1.27.0):

- `make ci-check` exit 0: fmt-check 291 файл, refusal-check 160, staticcheck
  9 пакетов без находок, vuln-check чисто, все публичные bundle'ы совпали.
- `make e2e` exit 0: 6 наборов `passed`.
- `make race` exit 0, гонок нет.
- `openspec validate --all --strict`: 23 passed, 0 failed.

Замеры, которых не было:

- `TestCLIProjectCloneJoinsWithoutForeignRunners`: клон, чей профиль объявляет
  три хоста при одном раннере, подключается `project init` и получает
  `missing_hosts: [codex-cli, codex-app]`; `runners add --host codex-cli`
  пишет раннер объявленного хоста (до правки — тот же `project_runner_missing`,
  что и у init); правленый руками раннер по-прежнему отказ, и отказ называет
  `project.runners.update` / `project.runners.add`.
- `TestCLIProjectRunnersUpdateCheckWritesNothing`: `--check` называет, что
  заменит, и оставляет файл байт в байт; следующий `update` заменяет; `add
  --check` отказан как чужой флаг.
- `TestDriveStopsBeforeAdmittingAProgram`: с границей драйвер не создаёт ни
  попытки, ни активной работы и оставляет шаг `ready`, тот же готовый шаг
  читается снова; без границы та же программа исполняется, как прежде.
- `TestProjectLaunchSummary…` (изменён): итог называет `environment_names` и не
  печатает ни одного значения; значения по-прежнему меняют
  `configuration_digest`.
- `TestSessionTask…` (изменён): форма `--all` вручает и пишет `task.json`,
  побайтно равный документу формы с одной попыткой; монитор (`SessionTasks`)
  по-прежнему не пишет ничего.
- Раннеры: прежний текст 0.13.32 заморожен двенадцатым вариантом, пины
  обновлены, `runners update` заменяет каждый released вариант.

Измерено попутно и записано в справочник: `stage_work` приходит только у
Run'ов, начатых сборкой с `core-state/31`; у прогона без ассистируемых шагов
состояние остаётся прежним, поля нет, а `--stop-before-program` работает всё
равно — граница про действие драйвера, не про ответ.
