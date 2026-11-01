# Задание агенту без контекста

Pri-Fly — Go-движок устойчивых сценариев: он выдаёт задания исполнителям и
ведёт запись того, что им было сказано и что они вернули. Он не вызывает
модели сам и не владеет промптами.

## Прочитай до первой правки

1. `AGENTS.md` в корне — обязательные правила репозитория.
2. `openspec/SOURCE-OF-TRUTH.md`, relevant capability specification и active
   OpenSpec change — где сегодня меняется правило и как оформить изменение.
3. `openspec/specs/delivery-roadmap/spec.md` — текущие границы продукта и
   roadmap; наличие определения не означает завершённую приёмку.
4. `openspec/specs/specification-governance/terms.md` перед изменением
   предметного понятия, API или терминологии.
5. `internal/flow/protocol.schema.json` перед изменением опубликованного
   контракта: источник истины — bytes рядом с кодом, не догадка агента.

OpenSpec управляет документацией и процессом Pri-Fly. Он не входит в runtime,
не заменяет user-facing YAML и не даёт права менять historical evidence.

## Правило, которое важнее скорости

**Не утверждай больше, чем проверил.** Неподтверждённая capability,
qualification или результат gate остаётся непроверенной. Неизвестный исход не
становится нулём или успехом. Если test не воспроизводит обещание, исправь test
или его имя и назови непокрытую границу.

Комментарии объясняют почему, а не пересказывают код. При обновлении
версированного state или public schema проверь сохранённые bundles и читателей;
стилистика не оправдывает изменение сохранённого JSON или смысла Run.

## Как вести срез

1. Из OpenSpec change и опубликованного контракта выведи точные границы.
2. Сначала добавь проверку отказа, затем положительный сквозной сценарий.
3. Выполни точечные tests затронутого поведения, freshness contracts и
   `git diff --check`. Полные ворота запускай только для product/release
   milestone либо прямого запроса владельца.
4. Отметь выполненную задачу в `tasks.md` active OpenSpec change. Не объявляй
   roadmap phase или product gate закрытым без назначенного evidence.
5. Коммит объясняет почему принятое решение сохраняет контракт; затем push.

Всегда запускай команды из явного target worktree или передавай ему `-C`.
Зелёный результат в соседнем дереве не доказывает ничего о редактируемом.

## Композиция сценария

Композиция workflow выполняется в compiler. Импорты читают source exports,
а `extends` наследует effective graph и extensions. Несколько родителей
требуют единственной ближайшей exact основы. Общие contributions включаются
один раз; пересечение дополнений требует явного `arrange`.

```text
parents + source imports → ordered layers → consumer extend → native graph
```

`prifly-project-workflow/2` и `prifly-extension/3` не меняют закреплённые Runs.
Required/not-required/unknown определяет вызываемый workflow по task и candidate evidence.
Core не определяет UI/UX по имени файла. Synthetic engine tests не квалифицируют браузер.

## Не делай

- Не создавай новый синоним, тип или JSON field без нового предметного смысла.
- Не переписывай historical evidence, manifests, старые public bundles или
  release record ради согласования с текущим текстом.
- Не расширяй срез молча. Внешний дефект опиши отдельным change.
- Не выдавай проверку документа за qualification продукта и не обещай
  самопробуждение: срок замечается, когда authority снова проверит состояние.

## Как разговаривать с владельцем

Пиши по-русски, простыми словами, начиная с результата. Отчёт называет:
что сделано, что намеренно не сделано и почему, точный результат ворот.

Для opt-in nested extensions прочитай active change
`add-nested-project-extension-points`. Package author фиксирует anchor, inputs,
routes и consumer ports в `extension-points.yaml`; project author выбирает
implementation и explicit outputs в `extend.yaml` `/4`. Reader получает новый
carrier только по объявленному binding. Публикация report не расширяет effects
reader и не доказывает реальный provision либо release.


Совместные composition и points определены в main source sets
`workflow-and-context`, `cli-protocol`, `published-contracts` и
`specification-governance`. История выпуска Core v0.13.85 находится в archived
`2026-11-01-unify-workflow-composition-and-extension-points`.
Root выбирает extension `/5` либо folder `/3` явно. Проверяйте source owner до flattening, public point
export и изоляцию compiled variants. Старый `/4` остаётся на прежнем пути.
Группа в точке с consumers получает отказ; explicit typed Call передаёт carrier.
Source candidate и опубликованный replay имеют отдельное evidence.
