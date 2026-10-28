package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type StringList []string

func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *StringList) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*s = StringList{}
		return nil
	case []byte:
		return json.Unmarshal(x, s)
	case string:
		return json.Unmarshal([]byte(x), s)
	default:
		return fmt.Errorf("unsupported type %T for StringList", v)
	}
}

type Option struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type OptionList []Option

func (o OptionList) Value() (driver.Value, error) {
	if o == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]Option(o))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (o *OptionList) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*o = OptionList{}
		return nil
	case []byte:
		return json.Unmarshal(x, o)
	case string:
		return json.Unmarshal([]byte(x), o)
	default:
		return fmt.Errorf("unsupported type %T for OptionList", v)
	}
}

type ReportMap map[string]any

func (r ReportMap) Value() (driver.Value, error) {
	if r == nil {
		return "{}", nil
	}
	b, err := json.Marshal(map[string]any(r))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (r *ReportMap) Scan(v any) error {
	switch x := v.(type) {
	case nil:
		*r = ReportMap{}
		return nil
	case []byte:
		return json.Unmarshal(x, r)
	case string:
		return json.Unmarshal([]byte(x), r)
	default:
		return fmt.Errorf("unsupported type %T for ReportMap", v)
	}
}
