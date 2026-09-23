## Пилотное read-only наблюдение, 2026-09-23

Project `SMSPlace/backend` использует authority `.prifly-sandbox-authority` и установленный `/Users/sh/.local/bin/prifly`. Проверка выполнялась только командами `claim list`, `run status`, `run next` и чтением SQLite в `mode=ro`; команды допуска, освобождения и изменения Run не вызывались.

- Run `run:5c6fd74e1b353f7b0f8461d7cd71b81194b299a7847d8c4e8f52a4b5817d82b7` создан командой `command:5072e237cf247f30573432a7967bcfa7`; его claim `claim:0b6de7b3bdf6c59a4ff38a13b8b710d2ccecdc1abf6a79f99ea31cdcbe324672` активен и привязан к этому Run. На момент чтения Run был `running`, `run next` отвечал `idle`.
- Run `run:5b79e266cb0a98597d08bb33223d26a85c71af3ba508d45ca7a6c8256992f9da` создан командой `command:d6c0d7d53f2dc2f520d966e568b4a174`; его claim `claim:9337ca7f76092885d961d5cd64c4ec0375f152ec2e53ce80cbb6f90d20ad4beb` был создан 2026-09-23 01:53 UTC и освобождён 2026-09-23 10:04 UTC без привязки к Run. На момент чтения Run был `ready`, `run next` называл stage `plan`. Освобождённый claim нельзя молча вернуть ему.

Это наблюдение подтверждает связь двух сохранённых Run с их Project claims по exact стартовым командам, но не является живым доказательством нового бинарника. Для второго Run потребуется отдельное безопасное recovery решение либо новый запуск; он не должен продолжаться на чужом worktree.
