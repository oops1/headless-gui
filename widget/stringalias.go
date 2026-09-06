// stringalias.go — псевдонимы ключей перевода.
//
// Штатные диалоги (MessageBox, выбор файла, ввод, прогресс) переводятся своими
// ключами «dlg.*». Приложение, у которого есть собственная таблица строк, может
// переопределить любой из них обычным RegisterStrings — это работало всегда.
//
// Но у приложения свои ИМЕНА ключей: «btn.ok», «app.cancel», «file.open». Чтобы
// диалоги говорили из той же таблицы, приходилось дублировать значения под
// вторым именем и следить, чтобы переводы не разъехались.
//
// Псевдоним связывает ключ движка с ключом приложения: значение остаётся в
// одной таблице, под одним именем, и язык у окна получается один.
package widget

import "sync"

var (
	aliasMu      sync.RWMutex
	stringAlias  map[string]string
	aliasEnabled bool // быстрый выход из Tr, пока псевдонимов нет
)

// AliasStrings связывает ключи движка с ключами приложения.
//
// Ключ карты — ключ движка («dlg.ok»), значение — ключ в таблице приложения
// («btn.ok»). Пустое значение снимает псевдоним.
//
//	widget.AliasStrings(map[string]string{
//	    "dlg.ok":     "btn.ok",
//	    "dlg.cancel": "btn.cancel",
//	})
//
// Псевдоним СТАРШЕ собственного перевода ключа: оба — осознанные действия
// приложения, и то, что названо позже и конкретнее, должно побеждать. Если
// перевода нет и по псевдониму, берётся обычный перевод самого ключа — то
// есть встроенный. Хуже, чем было, не станет ни при каком раскладе.
func AliasStrings(pairs map[string]string) {
	if len(pairs) == 0 {
		return
	}
	aliasMu.Lock()
	if stringAlias == nil {
		stringAlias = make(map[string]string, len(pairs))
	}
	for k, v := range pairs {
		if v == "" {
			delete(stringAlias, k)
			continue
		}
		stringAlias[k] = v
	}
	aliasEnabled = len(stringAlias) > 0
	aliasMu.Unlock()

	// Открытые диалоги и подписанные строки должны перечитать переводы:
	// смена псевдонима меняет видимый текст ровно так же, как смена языка.
	notifyLanguageChanged()
}

// AliasString связывает один ключ движка с ключом приложения.
func AliasString(engineKey, appKey string) {
	AliasStrings(map[string]string{engineKey: appKey})
}

// ClearStringAliases забывает все псевдонимы (нужен в тестах).
func ClearStringAliases() {
	aliasMu.Lock()
	stringAlias = nil
	aliasEnabled = false
	aliasMu.Unlock()
	notifyLanguageChanged()
}

// aliasFor возвращает ключ приложения для ключа движка.
func aliasFor(key string) (string, bool) {
	aliasMu.RLock()
	if !aliasEnabled {
		aliasMu.RUnlock()
		return "", false
	}
	v, ok := stringAlias[key]
	aliasMu.RUnlock()
	return v, ok
}
