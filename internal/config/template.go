package config

import _ "embed" // the config template

// SchemaURL is the JSON Schema of config.toml (schema/config.schema.json in the repo). Editors
// with Taplo read it from the #:schema line that Template starts with.
const SchemaURL = "https://raw.githubusercontent.com/0mega24/tandem/main/schema/config.schema.json"

// template holds the commented [generate] tables with their defaults; tools/gendocs writes it.
//
//go:embed zz_generated_template.toml
var template string

// Template is the file that tdm config init writes: the #:schema line, then the template.
func Template() string { return "#:schema " + SchemaURL + "\n\n" + template }
