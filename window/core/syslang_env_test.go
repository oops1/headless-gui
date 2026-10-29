//go:build !windows

package core

import (
	"reflect"
	"testing"
)

// На Linux и macOS SystemLanguages читает переменные среды процесса.
func TestSystemLanguages_FromProcessEnv(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	t.Setenv("LANGUAGE", "ru:en_US")
	want := []string{"ru", "en-US", "ru-RU"}
	if got := SystemLanguages(); !reflect.DeepEqual(got, want) {
		t.Errorf("SystemLanguages() = %q, ждал %q", got, want)
	}

	t.Setenv("LC_ALL", "C")
	if got := SystemLanguages(); got != nil {
		t.Errorf("при LC_ALL=C SystemLanguages() = %q, ждал пустой список", got)
	}
}
