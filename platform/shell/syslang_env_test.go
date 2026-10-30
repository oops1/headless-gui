//go:build !windows

package shell

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
	if got := systemLanguages(); !reflect.DeepEqual(got, want) {
		t.Errorf("systemLanguages() = %q, ждал %q", got, want)
	}

	t.Setenv("LC_ALL", "C")
	if got := systemLanguages(); got != nil {
		t.Errorf("при LC_ALL=C systemLanguages() = %q, ждал пустой список", got)
	}
}
