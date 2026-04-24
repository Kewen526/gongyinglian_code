package model

import (
	"database/sql/driver"
	"fmt"
	"strconv"
)

// Float3 is a float64 that always serializes to JSON with exactly 3 decimal places.
type Float3 float64

func (f Float3) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.3f", float64(f))), nil
}

func (f *Float3) UnmarshalJSON(data []byte) error {
	v, err := strconv.ParseFloat(string(data), 64)
	if err != nil {
		return err
	}
	*f = Float3(v)
	return nil
}

// Value implements driver.Valuer so GORM can write Float3 to the database.
func (f Float3) Value() (driver.Value, error) {
	return float64(f), nil
}

// Scan implements sql.Scanner so GORM can read decimal columns into Float3.
func (f *Float3) Scan(value interface{}) error {
	if value == nil {
		*f = 0
		return nil
	}
	switch v := value.(type) {
	case float64:
		*f = Float3(v)
	case float32:
		*f = Float3(v)
	case int64:
		*f = Float3(v)
	case []byte:
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return fmt.Errorf("Float3 scan []byte: %w", err)
		}
		*f = Float3(n)
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("Float3 scan string: %w", err)
		}
		*f = Float3(n)
	default:
		return fmt.Errorf("Float3 scan: unsupported type %T", value)
	}
	return nil
}
