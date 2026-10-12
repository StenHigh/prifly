## Context

Мотивация и границы — [proposal.md](proposal.md). Fixtures уже доступны в
[project-launch-advice](../../../test/fixtures/project-launch-advice/README.md).
Прежний Claude finite trace остановился на provider 429 до native вопроса;
новое наблюдение должно иметь собственную сессию, ответы и evidence.

## Goals / Non-Goals

**Goals:** воспроизводимый реальный UI-диалог и независимое подтверждение
отсутствия зависимых действий и изменения repository/authority до ответа.

**Non-Goals:** mock доступности tools, повторное использование прежнего human
ответа, смена provider/model/mode ради UI, product workflow execution или
автоматическое исправление runner в ходе приёмки.

## Decisions

1. Начинать только после отдельного запроса владельца и восстановления доступа.
   Подготовить новый candidate из текущего `main`, записать commit и binary/
   runner hashes, создать новые `finite` и `pages` fixtures публичным setup.
   Исторический resume служит evidence блокировки, а не fresh acceptance.
2. Человек отвечает на declared finite вопрос непосредственно в Claude UI.
   Значение повторной анкеты должно совпадать с выбранным catalog value;
   destination `mode` уже связывает input и не передаётся второй раз.
   Test-driver Next/Back/Defer допускается для навигации с явной пометкой его
   происхождения; такой ответ не выдаётся за новое human предпочтение.
3. Лимит берётся из реально предоставленного `AskUserQuestion` contract.
   Проверить все 15 ID и tool arguments по фактическому лимиту, возврат,
   отказ без выбранного ID. Заданные короткие Codex pages из архива не заменяют
   эту проверку. Отдельная fixture нужна для реального dismissal без ответа.
4. Независимый harness снимает все paths (включая hidden/ignored), типы, права,
   SHA-256 bytes и symlink targets до native вопроса и после него. Снимки и
   logs хранятся вне repository/authority. Atime исключается; неподдержанные
   ACL/xattrs и промежуточные изменения явно остаются пределом наблюдения.
   Transcripts дополняют snapshots: отсутствие итоговой дельты само по себе
   не доказывает отсутствие кратковременного mutation.
5. PASS относится только к фактически наблюдённому случаю; 429, недоступный
   tool или невозможность реального dismissal дают BLOCKED/NOT OBSERVED.
   Записывать evidence здесь, не переписывая архив реализации. При обнаружении
   дефекта сначала воспроизведение и отдельное согласование repair scope.

## Risks / Trade-offs

- [Provider снова вернёт 429] → оставить конкретный случай BLOCKED, не выбирать
  ответ и не обходить настройки; остальные независимые случаи можно проверить.
- [Preselection/receipt/dismissal принят за ответ] → отдельный trace actual
  reply и контроль до него; отмена/timeout не разрешает prepare/start.
- [Пагинация проверена только текстом] → реальные native calls и переходы,
  exact ID coverage, проверка arguments по фактическому host contract.
- [Снимки скрывают короткое изменение или содержат секрет] → audit tool calls,
  минимальные отфильтрованные traces без reasoning, credentials и binary dump;
  ограничение snapshot-интервала записать явно.

## Migration Plan

Миграция продукта не требуется. После завершения новых проверок зафиксировать
их результаты в этом change и архивировать его отдельно; formal release и
roadmap gates оцениваются своим процессом.
