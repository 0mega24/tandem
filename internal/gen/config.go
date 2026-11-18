package gen

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"
)

// DecodeConfig decodes a [generate.<id>] table over the defaults in def. The struct fields use
// toml tags, which also name the keys in a YAML config. An unknown key is an error, so a typo is
// found.
func DecodeConfig[T any](raw map[string]any, def T) (T, error) {
	cfg := def
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "toml",
		ErrorUnused:      true,
		WeaklyTypedInput: false,
		Result:           &cfg,
	})
	if err != nil {
		return def, err
	}
	if err := dec.Decode(raw); err != nil {
		return def, fmt.Errorf("decode generator config: %w", err)
	}
	return cfg, nil
}
