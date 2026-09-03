# Безопасность поставки Pri-Fly

Pri-Fly F1 — **core-local/cooperative**, один доверенный OS owner. Пользовательские executables имеют права этого пользователя. Immutable refs, scoped API и process ownership защищают от ошибок протокола; они не являются sandbox от процесса с прямым доступом к тем же файлам/UID. Не исполняйте недоверенные scripts, LLM-generated shell без проверки или команды с production credentials под видом изолированного workflow.

## Куда сообщать

Используйте **private vulnerability reporting** репозитория [StenHigh/prifly](https://github.com/StenHigh/prifly/security/advisories/new) (Security → Report a vulnerability); адресат — владелец `StenHigh`. Скрипты выпуска ничего не публикуют автоматически.

Отдельный публичный security email/bug bounty владелец не назначал; не выдумываем такой канал. Если у вас нет доступа к проекту, сначала согласуйте закрытый канал с владельцем. Не публикуйте secrets, `.prifly/installation.json`, bearer tokens, raw production artifacts или полную SQLite в публичных issues/чатах.

## Учётные данные выпуска

`PRIFLY_RELEASE_SIGNING_KEY` — secret environment `release` в GitHub Actions:
private Ed25519 key только для signing manifest. Он доступен единственному
publication job и только после approve владельца в этом environment.
`PRIFLY_RELEASE_PUBLIC_KEY` — repository variable; он встраивается в binary
при сборке и проверяет подпись при `prifly update`. Publisher credential —
job-scoped `GITHUB_TOKEN` с `contents: write` только в publication job;
долгоживущего publish token нет, а создание release tags `v*` ограничено
ruleset владельца. Не выводите ни одно значение в log, не используйте private
key локально и немедленно rotate его при подозрении на раскрытие.

Полезные данные: version/binary SHA, OS/arch, semantics/trust profile, минимальный sanitized workflow, command/Run/Attempt IDs, ожидаемое и фактическое поведение. Raw evidence передавайте только по согласованному закрытому каналу. Hash или скриншот не заменяет воспроизводимый сценарий, если его можно безопасно подготовить.

## Ограничить проблемную сборку

1. Прекратите **новые** Start/Drive/admissions. Сохраните version, SHA и status/receipts. Не удаляйте историю.
2. Для ещё живого собственного driver используйте pause либо cancel по задаче. Сообщение об отправке сигнала не означает settlement. При uncertainty не запускайте повтор.
3. Сохраните authority и внешние эффекты для разбора. Не редактируйте snapshots, stop generations, dedup receipts или lock-файлы вручную.
4. Соберите/получите исправленный проверенный binary, сверьте checksum, notices, capabilities и compatibility evidence. Сначала проверьте на отдельной пустой authority и тестовых данных.
5. `prifly update` меняет только официально установленный binary после проверки signed release; он не меняет authority, package, configuration или уже загруженные bytes работающего driver. Не начинайте новую работу другой версией против той же authority, пока не понятен статус текущей; automatic downgrade, force-upgrade и удаление несовместимых событий не поддержаны.

Pri-Fly не содержит remote kill switch, network control plane или автоматического исправления state. `prifly update` запускается только явно, а первая bootstrap-установка доверяет official GitHub HTTPS Release asset; не заменяйте этот адрес branch, job artifact или сторонним URL. Полный rollback всех файлов по прежнему пути не распознаётся без внешнего fencing и не является разрешённым recovery. Новая версия продукта не снимает прежние restrictions сама.
