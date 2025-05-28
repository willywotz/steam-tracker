package steamtracker

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSON json.RawMessage

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = JSON{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("unsupported type for JSON: %T", value)
	}

	result := json.RawMessage{}
	err := json.Unmarshal(bytes, &result)
	*j = JSON(result)
	return err
}

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	bytes, err := json.RawMessage(j).MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(bytes), nil
}
