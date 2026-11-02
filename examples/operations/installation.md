[Индекс](index.md) · [Новый запуск](launch.md)

# Установка и обновление

## Первая установка и clone

Поддержанный способ первой установки находится в [README](../../README.md).
Bootstrap скачивает assets официального GitHub Release и устанавливает binary в каталог пользователя без `sudo`.
Первая установка доверяет GitHub HTTPS и digest manifest. Работающий updater затем проверяет встроенным ключом подпись manifest.
Source build и скопированный binary без installation receipt не поддерживают self-update.

Для нового проекта сначала выберите нужный host. Для обычных программ host можно не задавать.

```sh
"$PRIFLY_BIN" project init --repository "$repository" --host codex-app --json
# Другие host IDs: codex-cli, claude-code. --host можно повторять.
# Отдельная authority: добавьте --state-root /absolute/path/outside/repository.
```

В свежем clone `project init` создаёт local configuration и authority, сохраняя shared profile.
Authority должна находиться вне repository. Для project launches её создаёт именно `project init`.
Обычный `init --profile core-workflow/1` создаёт другой набор configuration и не заменяет project bootstrap.
Existing `local.yaml` и custom runner вызывают конфликт вместо перезаписи.
Runner отсутствующего host добавляйте явным `project runners add --host HOST --repository DIR`.
`runners update` сообщает `missing_hosts`; эта команда не устанавливает отсутствующий host автоматически.

## Обязательная проверка перед новым Run

Эта процедура относится к существующему `prifly-run`, перед `project start`, `project continue` или `project recover`.
Прямые CLI-команды сохраняют явную семантику и не обновляют установку скрыто.
Просмотр, drive и текущий Attempt существующего Run не требуют проверки новой установки.

```text
local.yaml -> binary update -> version/capabilities -> workflow updates
                                                          |
                                                          v
runner/manual update -> read new skill/PROJECT -> questionnaire -> prepare
```

1. Прочитайте exact executable и authority root из machine-local configuration.
2. Выполните `"$PRIFLY_BIN" update --json`.
3. Прочитайте `version --json` и `capabilities --json` по тому же executable path.
4. Для каждого уже установленного package с `origin` выполните `project workflows update NAME --repository DIR --json`.
5. Проверьте `project runners update --check --repository DIR --json` и примените необходимый update без `--check`.
6. Перечитайте обновлённый `SKILL.md`, `PROJECT.md` и список launches перед окончательным выбором.

`update` сразу проверяет и применяет новую signed stable version. Флага `update --check` нет.
`updated:false` означает успешную проверку без замены; ошибка сети или подписи не означает актуальность.
Сохраните в кратком отчёте previous/current binary version, updater source и old/new workflow commits.
Проверка относится к одной подготовке запуска. Не повторяйте её после каждого drive, task или accepted report.
При перечитывании нового skill используйте уже выполненную проверку этой подготовки, чтобы не создать рекурсивный update loop.
Другой запрос запуска либо изменение проверяемых sources требует новой проверки.

Список package names берите из `packages` project profile. `project workflows` перечисляет launches, а не все packages.
Не выводите имя package из похожего launch ID. Установленный package может быть dependency без собственного launch.
Локальный авторский package без `origin` не имеет upstream update. Назовите его локальным и не придумывайте ему remote.
Если владелец требует внешнее подтверждение его версии, дождитесь такого подтверждения.

## Branch, tag, commit и каталог

`workflows update NAME` проверяет записанный `origin.ref`, а не произвольный «последний tag».
Для moving branch новый commit обновляется даже при прежнем авторском `package.version`.
Fixed tag или commit закрепляет линию исходников. Не меняйте его молча на другой ref.
Для catalog-installed package прочитайте актуальную запись исходного каталога через `workflows search NAME --catalog URL`.
Если запись теперь указывает новый release ref/commit, покажите переход и получите выбор владельца перед `--ref NEW_REF`.
Для прямого Git origin явно сообщите: проверен настроенный ref; обнаружение других releases этой проверкой не обещано.

```sh
"$PRIFLY_BIN" project workflows search QUERY --json
"$PRIFLY_BIN" project workflows search NAME --catalog CATALOG_URL --json
"$PRIFLY_BIN" project workflows update NAME --repository "$repository" --json
# Только после явного выбора новой линии:
"$PRIFLY_BIN" project workflows update NAME --ref NEW_REF --repository "$repository" --json
```

Поиск читает каталог и ничего не устанавливает. Для установки нужен явный выбор:

```sh
"$PRIFLY_BIN" project workflows add NAME --repository "$repository" --json
# Или named Git URL с явными --path DIR, --ref REF, --name NAME, --catalog URL.
```

Add копирует source folder и объявляет package/launches. Это ещё не seal, trust или выполнение программ.
Credentials берутся из credential helper или SSH пользователя. Не помещайте credentials в URL.
`workflows remove NAME` удаляет папку и её launches из project profile; прежние authority packages/Runs сохраняются.
Remove удаляет также project settings этой папки. Его нельзя использовать как автоматический обход failed update.

## Что сохраняет update

Workflow update сохраняет bytes `extend.yaml` и собственную папку `project/`.
Он отказывает при изменении upstream-owned файлов. При конфликте остановите новый запуск и сохраните local changes.
Новый upstream `extend.yaml` может отличаться: ответ сообщает `extend_upstream_changed`, но настройки проекта автоматически не заменяет.
Profile `/3` создаёт отдельный compiled variant для новых bytes. Legacy `/2` может отказать из-за прежней package identity.
Не мигрируйте marker `/2` автоматически ради успешного запуска.

Runner update сохраняет `PROJECT.md` и пользовательские файлы.
Поставляемые skill и references обновляются согласованно; изменённая управляемая страница вызывает конфликт.
Missing managed page восстанавливается явным update. Не удаляйте весь `references/` ради обновления.
Пользовательские файлы не становятся управляемыми только из-за нахождения в этой папке.

## Отказ и одновременное обновление runner

Updater сначала проверяет конфликты всех выбранных hosts, затем заменяет bundle каждого host целиком.
Ошибка файловой системы на позднем host может оставить предыдущие hosts уже обновлёнными.
Диагностика перечисляет `already updated hosts`; сообщите этот список и остановите подготовку запуска.
Повторный явный update проверяет фактические bytes и завершает оставшиеся обновления.

`.prifly-runner-update.lock` в repository блокирует второй updater. Read-only check также сообщает занятый lock.
Прерванный процесс может оставить lock; автоматического захвата устаревшего lock нет.
Перед восстановлением владелец проверяет отсутствие живого updater и сохранённые папки `.prifly-runner-stage-*`/`.prifly-runner-backup-*` рядом со skill.
При interrupted rename прежний bundle может оставаться в backup; сохраняйте его и пользовательские bytes до сверки metadata.
Не удаляйте managed pages или backup как автоматический обход отказа.
Ошибка прав создания lock отличается в диагностике от уже занятого lock.

## Machine-local настройки

```sh
"$PRIFLY_BIN" project local set --repository "$repository" --executable /absolute/path/to/prifly --json
"$PRIFLY_BIN" project local set --repository "$repository" --allow-executable shell=/bin/bash --json
"$PRIFLY_BIN" project local set --repository "$repository" --env APP_ENV=development --json
"$PRIFLY_BIN" project local set --repository "$repository" --env-from API_KEY=env:API_KEY --json
```

`--allow-executable`, `--env`, `--env-from` и `--model-profile` повторяются по объявленному формату.
Sources окружения: `env:VAR`, `file:/path`, `dotenv:/path:KEY`. Значения секретов не копируются в shared workflow.
Разрешённый executable не означает разрешение программы вне объявленного шага.
Изменение local configuration после seal не переписывает окружение прежнего Run.

## Когда остановиться

Network/signature failure, missing receipt, unsupported installation, modified workflow или custom runner останавливают новый запуск.
Назовите exact code, executable path и уже применённые частичные обновления.
Не заменяйте source build скачанным installer, не reset-ите дерево и не продолжайте на старой версии молча.
Проверка updates не создаёт Run, claim или trusted package и не выполняет программу workflow.

Далее: [выбор и prepare](launch.md), [конфликты](troubleshooting.md), [контракт поставки](../../openspec/specs/release-distribution/spec.md).
