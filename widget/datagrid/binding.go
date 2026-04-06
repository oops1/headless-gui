// Package datagrid — DataGrid и система Data Binding, совместимая с WPF.
//
// Binding связывает свойство UI-элемента с полем модели данных.
// Поддерживает:
//   - Path: путь к свойству через точку ("User.Name")
//   - Mode: OneWay, TwoWay, OneTime
//   - Converter: IValueConverter для преобразования значений
//   - StringFormat: форматирование строкового представления
package datagrid

import (
	"fmt"
	"reflect"
	"strings"
)

// ─── Binding Mode ──────────────────────────────────────────────────────────

// BindingMode определяет направление привязки данных.
type BindingMode int

const (
	// OneWay — модель → UI (по умолчанию).
	OneWay BindingMode = iota
	// TwoWay — модель ↔ UI.
	TwoWay
	// OneTime — однократное чтение при привязке.
	OneTime
)

// ─── IValueConverter ───────────────────────────────────────────────────────

// IValueConverter преобразует значение при биндинге (WPF IValueConverter).
type IValueConverter interface {
	// Convert преобразует значение модели для отображения в UI.
	Convert(value interface{}) interface{}
	// ConvertBack преобразует значение UI обратно для записи в модель.
	ConvertBack(value interface{}) interface{}
}

// ─── Binding ───────────────────────────────────────────────────────────────

// Binding описывает привязку свойства UI к свойству модели.
type Binding struct {
	// Path — путь к свойству через точку, например "User.Name".
	Path string
	// Mode — направление привязки (OneWay, TwoWay, OneTime).
	Mode BindingMode
	// Converter — опциональный конвертер значений.
	Converter IValueConverter
	// StringFormat — формат строки (fmt.Sprintf), например "%.2f".
	StringFormat string
}

// ─── PropertyAccessor ──────────────────────────────────────────────────────

// GetPropertyValue получает значение свойства по пути через точку.
// Поддерживает:
//   - Поля структуры: "Name", "Address.City"
//   - Указатели: автоматически разыменовывает *T
//   - Map[string]interface{}: "key.subkey"
//   - PropertyGetter интерфейс: вызывает GetProperty(name)
func GetPropertyValue(obj interface{}, path string) (interface{}, bool) {
	if obj == nil || path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	current := reflect.ValueOf(obj)

	for _, part := range parts {
		// Разыменовываем указатели
		for current.Kind() == reflect.Ptr || current.Kind() == reflect.Interface {
			if current.IsNil() {
				return nil, false
			}
			current = current.Elem()
		}

		switch current.Kind() {
		case reflect.Struct:
			field := current.FieldByName(part)
			if !field.IsValid() {
				// Попробуем метод
				method := reflect.ValueOf(obj).MethodByName(part)
				if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() >= 1 {
					result := method.Call(nil)
					current = result[0]
					continue
				}
				// Попробуем метод-геттер: Get + Part
				getter := reflect.ValueOf(obj).MethodByName("Get" + part)
				if getter.IsValid() && getter.Type().NumIn() == 0 && getter.Type().NumOut() >= 1 {
					result := getter.Call(nil)
					current = result[0]
					continue
				}
				return nil, false
			}
			current = field

		case reflect.Map:
			key := reflect.ValueOf(part)
			val := current.MapIndex(key)
			if !val.IsValid() {
				return nil, false
			}
			current = val

		default:
			// Проверим интерфейс PropertyGetter
			if pg, ok := current.Interface().(PropertyGetter); ok {
				v, ok := pg.GetProperty(part)
				if !ok {
					return nil, false
				}
				current = reflect.ValueOf(v)
			} else {
				return nil, false
			}
		}
	}

	if !current.IsValid() {
		return nil, false
	}
	return current.Interface(), true
}

// SetPropertyValue устанавливает значение свойства по пути через точку.
// Поддерживает структуры и map[string]interface{}.
func SetPropertyValue(obj interface{}, path string, value interface{}) bool {
	if obj == nil || path == "" {
		return false
	}

	parts := strings.Split(path, ".")
	current := reflect.ValueOf(obj)

	// Проходим до предпоследнего элемента пути
	for i := 0; i < len(parts)-1; i++ {
		for current.Kind() == reflect.Ptr || current.Kind() == reflect.Interface {
			if current.IsNil() {
				return false
			}
			current = current.Elem()
		}

		switch current.Kind() {
		case reflect.Struct:
			field := current.FieldByName(parts[i])
			if !field.IsValid() {
				return false
			}
			current = field
		case reflect.Map:
			key := reflect.ValueOf(parts[i])
			val := current.MapIndex(key)
			if !val.IsValid() {
				return false
			}
			current = val
		default:
			return false
		}
	}

	// Устанавливаем значение последнего поля
	lastPart := parts[len(parts)-1]

	for current.Kind() == reflect.Ptr || current.Kind() == reflect.Interface {
		if current.IsNil() {
			return false
		}
		current = current.Elem()
	}

	switch current.Kind() {
	case reflect.Struct:
		field := current.FieldByName(lastPart)
		if !field.IsValid() || !field.CanSet() {
			// Попробуем setter
			setter := reflect.ValueOf(obj).MethodByName("Set" + lastPart)
			if setter.IsValid() && setter.Type().NumIn() == 1 {
				setter.Call([]reflect.Value{reflect.ValueOf(value)})
				return true
			}
			return false
		}
		val := reflect.ValueOf(value)
		if val.Type().ConvertibleTo(field.Type()) {
			field.Set(val.Convert(field.Type()))
			return true
		}
		return false

	case reflect.Map:
		key := reflect.ValueOf(lastPart)
		current.SetMapIndex(key, reflect.ValueOf(value))
		return true
	}

	return false
}

// PropertyGetter — интерфейс для объектов, поддерживающих динамический доступ к свойствам.
type PropertyGetter interface {
	GetProperty(name string) (interface{}, bool)
}

// ─── Resolve Binding ───────────────────────────────────────────────────────

// ResolveBinding получает значение из объекта по binding-пути.
// Применяет Converter и StringFormat.
func ResolveBinding(b *Binding, item interface{}) string {
	if b == nil || item == nil {
		return ""
	}

	val, ok := GetPropertyValue(item, b.Path)
	if !ok {
		return ""
	}

	// Применяем конвертер
	if b.Converter != nil {
		val = b.Converter.Convert(val)
	}

	// Применяем StringFormat
	if b.StringFormat != "" {
		return fmt.Sprintf(b.StringFormat, val)
	}

	return fmt.Sprintf("%v", val)
}
