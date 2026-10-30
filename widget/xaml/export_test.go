package xaml

func LocListenerCount() int { return LanguageListenerCount() }

func LocItemCount() int {
	locItemsMu.Lock()
	defer locItemsMu.Unlock()
	return len(locItems)
}

func XAMLScopeCount() int {
	xamlScopesMu.Lock()
	defer xamlScopesMu.Unlock()
	return len(xamlScopes)
}
