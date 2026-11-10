[Индекс](index.md) · [Команды](commands.md)

# Основы CLI и пути

## Четыре разных пути

**Repository** — каталог проекта с `.prifly/project.yaml`.
**Authority** — отдельное хранилище Run, заданий и принятых результатов.
**Repository workspace** — рабочее дерево, которое authority выделила конкретному Run.
**Attempt workspace** — каталог конкретной попытки шага с `task.json`, контекстом и выходными слотами.

```text
project repository -> prepare -> authority Run -> repository workspace
                                      |
                                      v
                               Attempt workspace
                                      |
                                      v
                          typed report -> accepted outputs
```

Пути repository workspace и Attempt workspace берите из текущего `session task`, а не вычисляйте по имени Run.
В `.prifly/local.yaml` прочитайте `prifly_executable` и `authority_root` как YAML-данные. Не исполняйте этот файл через shell.
Дальше примеры используют переменные `PRIFLY_BIN`, `authority_root` и `repository` с этими точными значениями.
`RUN`, `ATTEMPT`, `DIGEST`, `VERSION`, `ID` и `FILE` — placeholders, а не реальные идентификаторы.

```sh
"$PRIFLY_BIN" version --json
"$PRIFLY_BIN" capabilities --json
"$PRIFLY_BIN" help session submit
"$PRIFLY_BIN" --project "$authority_root" run next "$RUN" --json
```

Глобальный `--project DIR` выбирает authority. `--repository DIR` выбирает исходники проекта у `project` команд.
`--json` возвращает один документ stdout. `--format text|json|csv` выбирает представление; CSV поддерживает структурированные отчёты.
`--help`, `help [TOPIC]` и `--version` не требуют открытия authority. Полный `help` показывает формы команд.
`schema` без имени перечисляет точные имена contracts. Имена чувствительны к регистру.

## Что означает результат команды

Exit `0` означает, что команда выполнена. Ожидание хоста или решения также может вернуть `0`.
Исход работы определяют `status`, `outcome` и `verdict`, а не exit driver.
Exit `2` — общий отказ или неверная форма; `3` — состояние/политика/CAS; `4` — запрет доступа.
Exit `5` — операция сейчас невозможна; `6` — authority требует обслуживания; `7` — прерывание клиента.
Ненулевой exit не является штатным handoff. Точные классы показывает `exit-codes` установленной версии.

Каждый отказ заканчивает stderr одним Problem-документом. Читайте его `code`, `violations` и `safe_next_actions`.
Перед Problem может находиться launch summary. Не смешивайте stderr со stdout при разборе JSON.
`safe_next_actions` — указатель на доступные операции; он не выдаёт разрешение на новый эффект.

## Как искать без исходников Core

1. Откройте строку задачи или симптома в этом индексе.
2. Прочитайте только нужный раздел страницы.
3. Уточните форму через `help`, `schema` или текущий `run next`.
4. Если ответа нет, сообщите версию, Problem code и пробел документации.

Не начинайте обычную операцию с поиска по Go-коду Pri-Fly.
Исходники целевого проекта читать можно, когда этого требует задание шага.
Разработка или диагностика самого Core остаётся отдельной допустимой задачей в пределах effects.

Нормативные правила принадлежат [карте источников](https://github.com/StenHigh/prifly/blob/v0.13.88/openspec/SOURCE-OF-TRUTH.md).
Справочник не расширяет полномочия и не обещает реализацию roadmap.
При установке внешние ссылки закрепляются на source ref поставки; локальные страницы доступны без сети.
