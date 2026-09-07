# Проверка локального монитора Run

2026-09-07. База GitHub: `b1c8e93`; отдельный Git worktree и ветка
`codex/local-run-monitor`. Среда проверки: Go 1.27.0, darwin/arm64.

## Реализованный срез

Каталог хранит только пути к authority в пользовательском config-каталоге.
Run и артефакты не копируются и не связываются через `ln`. Обнаружение ищет
принадлежащие пользователю installation, дедуплицирует физические каталоги,
показывает неполноту/ошибки и оставляет копии разных физических authority
с одинаковой identity отдельными источниками с предупреждением о конфликте.
Полный telemetry scan сохраняет прежние ограничения; монитор перечисляет
метаданные страницами и перечитывает изменившиеся Run с проверкой integrity.

Автозапуск подключён после успешного commit Start/Fork, до Drive. Project start
использует тот же callback, scheduler вызывает общий Start. Новая scheduler CLI
команда не добавлялась. Ошибка monitor не меняет receipt и не останавливает Run.
Обычный публичный View сохраняет redaction; локальный HTTP использует отдельный
`local-run-monitor/1` с исходной read version, закреплёнными данными и выбором
ветвей из журнала на срезе Run. Publication credentials и executable environment
не выводятся.

## Выполненные проверки

- `go test ./internal/local ./internal/runtime ./cmd/prifly ./internal/purity -run 'Monitor|RunsList|Fork|Start|Schedule|Neutral|Project|StoreRefuses|Glossary' -count=1` — PASS.
- `go test -race ./cmd/prifly ./internal/runtime ./internal/local -run 'TestMonitor|TestRunsList|TestChoiceDurableSelection|TestForkCreatesSeparate' -count=1` — PASS.
- После финальных изменений повторены затронутые monitor/choice tests и race-проверка CLI monitor — PASS.
- `make vet refusal-check fmt-check schemas-check` — PASS. Vet включает `CGO_ENABLED=0`; публичные schema bundles совпадают с исходными.
- `make build` — PASS; внешняя frontend dependency отсутствует.
- `node cmd/prifly/monitor_ui_test.cjs` — PASS: escaping, качество времени, parallel/choice/nested scopes/repeat/cycles, список изменений по manifests.
- `openspec validate add-local-run-monitor --strict`, проверки main specs `local-run-monitor` и `cli-protocol` — PASS.
- `git diff --check` — PASS; historical evidence и публичные bundles не изменены.

Go-проверки покрывают 205 Run в одном хранилище, поиск за первой страницей,
invalid cursor/date/sort, повреждённый snapshot, read access, несколько источников,
symlink alias, чужой owner, недоступную область, ошибку одного источника и
удаление другого. Проверен настоящий detached spawn двух monitor-процессов на
одном временном loopback-порту, а также порт чужого сервиса. Тестовый config
изолирован; машинный каталог пользователя не используется для тестовых записей.

## Наблюдения в браузере

Codex In-app Browser, localhost:7781, два тестовых authority:

- Один Run с двумя параллельными assisted reviewers, второй — простой Run для
  проверки обновления. Оба появились в общем списке.
- Схема показывает обе ветви, соединения, join и будущие стадии; переход в opus
  выбирает его WorkflowInvocation. Из схемы клавиатурой открыты StageActivation,
  затем StepInstance и Attempt. Инструкция `aif:context/improve-skill` отображена
  как исходный текст; переданный конверт, контекст и отсутствие результата видны.
- Brief содержит `<img src=x onerror=alert(1)>`: строка показана буквально,
  элементов `img` не создано, ошибок JavaScript не обнаружено.
- После обычного CLI Drive второй Run перешёл с `v1/e2` на `v2/e4` и из «Готов»
  в «Завершён». Раскрытый Brief остался открыт, положение страницы осталось
  `scrollY = 775.5`. Перезагрузка прямой ссылки восстановила выбранный Run.
- Ошибка чтения тестовой копии была видна отдельно; после исправления прав
  тестового файла чтение восстановилось автоматически.
- После замечания о нечитаемых цветах диаграмма получила отдельную тёмную
  палитру: контраст текста узла 12,0:1, выбранного узла 8,3:1, подписей 11,9:1.
  Увеличены промежутки между колонками и разведены подписи пересекающихся
  переходов. В браузере повторно проверен план с двумя параллельными ветвями;
  сборка, `monitor_ui_test.cjs` и `git diff --check` прошли.
- Проверена системная тёмная тема. Светлая палитра и узкий layout заданы CSS;
  отдельная визуальная квалификация всех браузеров/экранов не заявлена.

## Границы

Обнаружение полное только для проверенной доступной локальной области; системные,
сетевые файловые системы и недоступные каталоги отмечаются отдельно. Полный
обход всех дисков этой машины и нагрузочная квалификация очень больших историй
не выполнялись. Первичная индексация и большие чтения могут занять больше
периода опроса; UI показывает поиск/индексацию и ошибки, а не обещает полноту.

Переписка чатов, вызовы инструментов, shell stdout/stderr и новые host collectors
отложены по согласованному scope. Файлы показываются по сохранённым деревьям;
нет однозначной входной/выходной пары — нет утверждения о полном списке изменений.
Полный `make check`, все платформенные runtime gates, release и P2 qualification
этой проверкой не заявлены. Основной checkout другого агента не редактировался.
