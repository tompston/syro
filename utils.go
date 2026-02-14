package syro

import "encoding/json"

type Decoder struct {
	JSON string
}

func NewDecoder(v any) (*Decoder, error) {
	json, err := json.Marshal(&v)
	if err != nil {
		return nil, err
	}

	return &Decoder{JSON: string(json)}, nil
}

func (d *Decoder) JsonIndent() (string, error) {
	var i any
	if err := json.Unmarshal([]byte(d.JSON), &i); err != nil {
		return "", err
	}

	pretty, err := json.MarshalIndent(i, "", "  ")
	return string(pretty), err
}
