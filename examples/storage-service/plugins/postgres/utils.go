// Package postgres
package postgres

import (
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
)

func scanRow(rows pgx.Rows, fields []pgproto3.FieldDescription) (map[string]any, error) {
	values := make([]any, len(fields))
	valuePtrs := make([]any, len(fields))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	row := make(map[string]any, len(fields))
	for i, field := range fields {
		row[string(field.Name)] = values[i]
	}

	return row, nil
}

func scanRows(rows pgx.Rows) ([]map[string]any, error) {
	fields := rows.FieldDescriptions()
	var result []map[string]any

	for rows.Next() {
		row, err := scanRow(rows, fields)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if result == nil {
		return []map[string]any{}, nil
	}

	return result, nil
}
