package tests

import (
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestMain устанавливает детерминированный in-memory буфер обмена на время
// прогона тестов. Иначе тесты буфера обмена (напр. TestTextInput_PasswordMode_NoCopy)
// зависят от РЕАЛЬНОГО системного буфера — глобального состояния ОС, которое
// меняют другие тесты/процессы, из-за чего тест «флакает».
func TestMain(m *testing.M) {
	widget.UseMemoryClipboard()
	os.Exit(m.Run())
}
