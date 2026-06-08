package widget

// validation.go — проверка ввода в стиле WPF (IDataErrorInfo + ValidatesOnDataErrors).
//
// Модель DataContext может реализовать DataErrorInfo (аналог System.ComponentModel
// IDataErrorInfo). Для TwoWay-привязок с ValidatesOnDataErrors=True движок после
// записи значения в модель спрашивает у неё текст ошибки для данного свойства и
// переводит виджет в состояние ошибки (красная рамка + подсказка с текстом).
//
// Пример модели:
//
//	func (m *Form) DataError(prop string) string {
//	    if prop == "Age" {
//	        if m.Age < 0 || m.Age > 150 { return "Возраст должен быть 0..150" }
//	    }
//	    return ""
//	}
//
// XAML:
//
//	<TextBox Text="{Binding Age, Mode=TwoWay, ValidatesOnDataErrors=True}"/>

// DataErrorInfo — аналог WPF IDataErrorInfo: возвращает текст ошибки для
// свойства propertyName или "" если значение корректно.
type DataErrorInfo interface {
	DataError(propertyName string) string
}

// ValidationAware реализуется виджетами, умеющими показывать ошибку валидации
// (красная рамка и т.п.). Пустая строка снимает состояние ошибки.
type ValidationAware interface {
	SetValidationError(msg string)
	ValidationError() string
}

// setValidationState применяет/снимает состояние ошибки у виджета, если он это
// поддерживает. Также отражает текст ошибки в ToolTip.
func setValidationState(w Widget, msg string) {
	if va, ok := w.(ValidationAware); ok {
		va.SetValidationError(msg)
	}
}
