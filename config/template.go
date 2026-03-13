package configtemplate

import _ "embed"

//go:embed template.yaml
var templateYAML string

func TemplateYAML() string {
	return templateYAML
}
