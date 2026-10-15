## Context

См. proposal.md. Каталог уже хранит compact RunSummary и обновляет изменённые
Run по revision. Таблица и details используют один HTTP server; filter/page и
scroll сохраняются в клиенте. Связи представлены `fork.source_run_id` и reason.
Admission waiting хранится отдельно от Run, поэтому независимое чтение может
дать неверную причину для новой revision. Браузер доступен локально через
Playwright; проверка HTML-строк недостаточна для E1-A8.

## Goals / Non-Goals

**Goals:** компактное достоверное чтение, переходы в существующие details,
поддержка generic workflows и нескольких активных ветвей.

**Non-Goals:** новая task identity, queue/scheduler, hardcoded стадии,
изменение исполнения карточкой, host telemetry и переписывание старых схем.

## Decisions

1. Добавить optional локальную board-проекцию в RunSummary. Это additive
   локальный contract монитора; public RunView/schema bytes не меняются.
   Отдельный fetch всех RunView потребовал бы чтения artifacts и definitions
   на каждую карточку, поэтому не используется.
2. Read-only storage helper читает Run snapshot и admission waiter в одной
   SQLite read transaction. Runtime валидирует snapshot обычными инвариантами.
   При изменении source nonterminal summaries перечитываются даже при прежней
   RunVersion: admission может измениться независимо от Run.
3. Категория выбирается детерминированно: terminal; остановка/uncertainty;
   pending decision; подтверждённая работа; admission waiting; awaiting host
   или signal; ready/control/acceptance; неизвестное. Все активные стадии и
   отдельные причины остаются в карточке независимо от выбранной колонки.
   `pending` не становится фактически начатым исполнением.
4. Доска имеет одну карточку на exact Run. Исходный partial находится в
   terminal, его продолжение — в своей текущей категории; ссылки показывают
   записанную историю. Независимые forks не сливаются. Альтернатива скрывать
   предков под «текущей задачей» потребовала бы новой identity и неоднозначного
   выбора одной ветви при повторном continue.
5. Общая пагинация ограничивает страницу карточек, а колонки явно сообщают,
   что показывают эту страницу. Фильтры применяются ко всему каталогу; дочерние
   Run доступны ссылками и в сохранённом табличном виде. UI экранирует все
   записанные строки и восстанавливает навигацию существующим механизмом.

## Risks / Trade-offs

- [Run revision не меняется при admission waiting] → nonterminal summary
  обновляется вместе с source и читает waiter атомарно с snapshot.
- [Карточка содержит несколько ожиданий] → приоритет категории фиксирован,
  все причины и parallel-ветви остаются видимыми.
- [Источник недоступен] → существующая source error и stale-пометка, остальные
  authority продолжают отображаться.
- [Большая история] → metadata paging без полного artifacts scan; проверка
  последующей страницы и поиска по Run вне первых 200.

## Migration Plan

Изменение additive: старые Runs читаются без rewrite/migration. Откат UI
возвращает прежние виды; optional summary fields не меняют сохранённые данные.
Источник норм и опубликованные state/read bundles остаются прежними.
