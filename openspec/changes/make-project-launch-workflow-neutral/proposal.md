## Why

Pri-Fly должен одинаково удобно запускать обычные программы, работу с ИИ и
смешанные сценарии. Ядро уже имеет нужные исполнители, но Project launch
безусловно требует Git, host и RunBrief; разные настройки одного package
сталкиваются при импорте, а универсальный runner содержит правила AIF.

## What Changes

- Сначала разделить авторскую версию package и точную локальную сборку:
  разные profiles, настройки и context bytes сосуществуют в одной authority,
  прежние Runs и запрет подмены сохранены.
- Затем дать Project workflow самостоятельный путь в обычной папке: YAML,
  typed inputs, локальные команды и результат без Git, AI host и фиктивной
  задачи. Переиспользовать существующий local-process executor.
- Сделать Git Workspace и host необходимыми только для соответствующей работы.
  Машинные пути к программам оставить локальными, переносимые объявления — в YAML.
- Убрать из общего runner правила improve/review и количества рецензентов.
  Анкета показывает объявления выбранного сценария, включая возможность
  заранее ответить на runtime-решения и увидеть границы автономного запуска.
- Принимать изменения по трём путям: без ИИ; команда → assisted task → команда;
  внешний AIF. Совместимость AIF проверять в его repository, не возвращая его
  graph, skills или частные правила в Pri-Fly.

## Capabilities

### New Capabilities

Нет: развиваем существующие contracts, не создаём новый движок или plugin framework.

### Modified Capabilities

- `product-model`: основание обычного Run без обязательного предметного RunBrief.
- `domain-execution`: Project вне Git и отсутствие не объявленного brief в lock.
- `workflow-and-context`: identities сборок, нейтральный versioned Project
  profile, optional hosts и декларативная настройка существующих executors.
- `cli-protocol`: независимый от Git/AI запуск, подготовка host только по
  необходимости, нейтральный runner и предзапусковое представление решений.
- `run-decisions`: общая анкета предответов и честная граница local-owner trust.
- `runtime-resources`: отдельные exact execution bindings запуска без
  подмены обычных inputs и общей конфигурации authority.
- `quality-and-acceptance`: сквозная проверка Project launch без отраслевых
  зависимостей и сосуществования вариантов в одной authority.

## Impact

Это план изменения продукта, а не только документации. Основная работа —
Project CLI/compiler, YAML schemas, переносимые примеры и узкие regression
checks. Runtime затрагивается лишь там, где требуется общий contract
конфигурации; composition, storage guarantees и набор operators не заменяются.

Все перечисленные capabilities уже имеют единственные источники в
`openspec/specs/` согласно `SOURCE-OF-TRUTH.md`; ownership не переносится.
Текущие main specs остаются baseline до применения delta. Очередь в
`delivery-roadmap` ссылается на этот план; glossary и entry-point docs
актуализируются вместе с реализацией, не объявляя план текущей возможностью.

Новый authoring profile вводится versionedly. Чтение `/2`, прежние exact refs,
installed packages, frozen host runners и Runs сохраняются. Миграция общего
YAML — явная, не побочный эффект запуска или установки workflow.

## Non-goals

Не добавляем model routing, новый executor framework, интеграции GitHub/Jira,
автоматический разбор вопросов из prose, произвольную запись в общую папку
без resource contract, деревья skill dependencies или новый язык сценариев.
Не обещаем изоляцию агента от владельца с тем же OS identity. Не закрываем
формальную приёмку P1/P2 и не меняем историческое release evidence.
