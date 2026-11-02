[Индекс](index.md)

# Исходный ref development build неизвестен

Эту копию установил development binary без VCS build metadata, например с `-buildvcs=false`.
Он не содержит доказательства exact source commit. Ссылки на нормативные исходники поэтому не ведут на moving main или выдуманный tag.
Локальный справочник и примеры остаются доступны. Формы установленного контракта читайте через `prifly schema` и `help`.
Для разработки Core используйте явно выбранный checkout и его OpenSpec source map.
Публичная stable поставка закрепляет внешние ссылки на release tag своей версии.
`references/manual-version.json` указывает binary version, source ref и development status.
