package config

import _ "embed" // the config template

// SchemaURL is the JSON Schema of config.toml (schema/config.schema.json in the repo). Editors
// with Taplo read it from the #:schema line that Template starts with.
const SchemaURL = "https://raw.githubusercontent.com/0mega24/tandem/main/schema/config.schema.json"

// generateTemplate holds the commented [generate] tables with their defaults; tools/gendocs
// writes it from the generators.
//
//go:embed zz_generated_template.toml
var generateTemplate string

// coreTemplate shows the settings of Config other than [generate], with the values of Defaults.
// TestTemplateIsDefaults checks that they agree.
const coreTemplate = `# tandem config. Each setting shows its default. Remove the # of a line to change it.
# All settings: https://github.com/0mega24/tandem#configure

# [discovery]
# root_dir = "~/repos"       # the folder with your main clones
# max_depth = 2              # how many folder levels below root_dir hold repos

# [spaces]
# root_dir = "~/workspaces"  # tdm makes each space in <root_dir>/<name>

# [git]
# branch_prefix = ""         # the branch of a space is <branch_prefix><name>

# [always]                   # what every new space gets
# repos = []                 # repos linked into each space, not checked out on the branch
# files = []                 # files copied into the space folder
# secrets = []               # files such as ".env", copied from each main clone to its worktree
# run = ""                   # a program that runs after new, add, remove and delete

`

// Template is the file that tdm config init writes: the #:schema line, the core settings, then
// the generator settings.
func Template() string { return "#:schema " + SchemaURL + "\n\n" + coreTemplate + generateTemplate }
