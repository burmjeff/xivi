package dbutils

import (
	"database/sql"
	"strings"
)

func NewNullString(s string) sql.NullString {
	if len(s) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

/*func CustomMapper(field reflect.Type, column string) string {
	switch field.Kind() {
	case reflect.Slice:
		if field.Elem().Kind() == reflect.String {
			return column
		}
	}
	return strings.ToLower(column)
}*/

func CustomMapper(column string) string {
	return strings.Replace(column, "[]", "_array_", 1)
}
