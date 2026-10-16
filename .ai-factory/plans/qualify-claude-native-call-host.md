# Follow-up: live Claude Code для native Call extensions

Статус: **pending**, требуется доступная подписка/авторизованный Claude Code.
Создано 2026-10-17 по явному решению владельца исключить live Claude из приёмки
Core 0.13.78 и выполнить отдельную квалификацию позднее. Это не дефект движка
и не PASS по synthetic template tests. Никакой login/token в Git или чат не нужен.

Исходная приёмка: [архив #9](../../openspec/changes/archive/2026-10-17-add-project-extension-native-call/live-host-evidence.md).
Обнаруженный executable: Claude Code `2.1.287`, `claude auth status`: `loggedIn=false`.

- [ ] Проверить авторизацию и записать фактическую версию host, не сохраняя секреты.
- [ ] Использовать подписанный Core 0.13.78, source SHA `201d4428b60e6afc9aef84fc5197a3185cf2f20b`, linux/amd64 binary `sha256:60a6dd1ca0524b4927418a64311207b4d6739080a8f518374d1889726d68ab5f`; при другой платформе закрепить её verified release asset и binary hash отдельно.
- [ ] Подготовить отдельные pass/cancel fixtures через `test/e2e/verify-call-extension-live.py prepare --host claude-code`; реальные generated runner, task/context/input читает Claude, не coordinator.
- [ ] Success: реальный take, native template/digests/submit, accepted report/gate, downstream exact exports и settled succeeded в одном Run/claim/candidate.
- [ ] Control: controller отменяет child после take; Claude наблюдает blocked_child без outputs/submit, прекращает работу; collector подтверждает settled cancelled. Не считать native cancel доказательством остановки внешнего browser actor.
- [ ] Сохранить host version/command, identities/digests, scope, exit/control/settlement и traces в новом evidence. Исторический archive 0.13.78 не переписывать; ссылаться на него из нового результата.

W6 review/UIUX/browser/model consumer qualification остаётся независимой работой
потребителя, даже после успешного завершения этого follow-up.
