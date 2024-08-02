/*
Copyright 2022 SUSE.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//nolint:gochecknoglobals
package cloudinit

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

var (
	// defaultTemplateFuncMap is the default set of functions for the template.
	defaultTemplateFuncMap = template.FuncMap{
		"Indent": templateYAMLIndent,
	}

	// ignoredCloudInitFields is a list of fields that are ignored from additionalCloudInit when generating final configuration.
	ignoredCloudInitFields = []string{"runcmd", "write_files", "ntp"}
)

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)

	return strings.Repeat(" ", i) + strings.Join(split, ident)
}

const (
	defaultYamlIndent = 2
	cloudConfigHeader = `## template: jinja
#cloud-config
`

	filesTemplate = `{{ define "files" -}}
write_files:{{ range . }}
-   path: {{.Path}}
    {{ if ne .Encoding "" -}}
    encoding: "{{.Encoding}}"
    {{ end -}}
    {{ if ne .Owner "" -}}
    owner: {{.Owner}}
    {{ end -}}
    {{ if ne .Permissions "" -}}
    permissions: '{{.Permissions}}'
    {{ end -}}
    content: |
{{.Content | Indent 6}}
{{- end -}}
{{- end -}}
`

	commandsTemplate = `{{- define "commands" -}}
{{ range . }}
  - {{printf "%q" .}}
{{- end -}}
{{- end -}}
`
	sentinelFileCommand = `echo success > /run/cluster-api/bootstrap-success.complete`

	ntpTemplate = `{{ define "ntp" -}}{{ if . -}}
ntp:
  enabled: true
  servers:{{ range .}}
  - {{printf "%q" .}}
    {{- end -}}	
{{- end -}}
{{- end -}}
`
	arbitraryTemplate = `{{- define "arbitrary" -}}{{- range $key, $value := . }}
{{ $key -}}: {{ $value -}}
{{- end -}}
{{- end -}}
`
)

// BaseUserData is shared across all the various types of files written to disk.
type BaseUserData struct {
	Header                  string
	PreRKE2Commands         []string
	DeployRKE2Commands      []string
	PostRKE2Commands        []string
	WriteFiles              []bootstrapv1.File
	ConfigFile              bootstrapv1.File
	RKE2Version             string
	SentinelFileCommand     string
	AirGapped               bool
	AirGappedChecksum       string
	NTPServers              []string
	CISEnabled              bool
	AdditionalCloudInit     string
	AdditionalArbitraryData map[string]string
}

func generate(kind string, tpl string, data interface{}) ([]byte, error) {
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)
	if _, err := tm.Parse(filesTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse files template")
	}

	if _, err := tm.Parse(commandsTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse commands template")
	}

	if _, err := tm.Parse(ntpTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse ntp template")
	}

	if _, err := tm.Parse(arbitraryTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse arbitrary template")
	}

	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}

	return out.Bytes(), nil
}

func cleanupAdditionalCloudInit(cloudInitData string) (string, error) {
	m := make(map[string]interface{})

	if err := yaml.Unmarshal([]byte(cloudInitData), m); err != nil {
		return "", fmt.Errorf("failed to unmarshal additional cloud-init datad: %w, please check if you put valid yaml data", err)
	}

	// Remove ignored fields from the map
	for _, field := range ignoredCloudInitFields {
		delete(m, field)
	}

	bytesBuf := bytes.Buffer{}
	encoder := yaml.NewEncoder(&bytesBuf)
	encoder.SetIndent(defaultYamlIndent)

	if err := encoder.Encode(m); err != nil {
		return "", fmt.Errorf("failed to marshal additional cloud-init data: %w", err)
	}

	res := bytesBuf.String()
	if res == "{}\n" {
		return "", nil
	}

	return res, nil
}

func cleanupArbitraryData(arbitraryData map[string]string) error {
	// Remove ignored fields from the map
	for _, field := range ignoredCloudInitFields {
		delete(arbitraryData, field)
	}

	if len(arbitraryData) == 0 {
		return nil
	}

	m := make(map[string]interface{})

	kind := "arbitrary_prepare"
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)

	if _, err := tm.Parse(arbitraryTemplate); err != nil {
		return errors.Wrap(err, "failed to parse arbitrary keys template")
	}

	t, err := tm.Parse(`{{template "arbitrary" .AdditionalArbitraryData}}`)
	if err != nil {
		return errors.Wrap(err, "failed to parse arbitrary template")
	}

	var out bytes.Buffer
	if err := t.Execute(&out, BaseUserData{
		AdditionalArbitraryData: arbitraryData,
	}); err != nil {
		return errors.Wrapf(err, "failed to generate %s template", kind)
	}

	if err := yaml.Unmarshal(out.Bytes(), m); err != nil {
		return fmt.Errorf("failed to unmarshal arbitrary cloud-init data: %w, please check if you put valid yaml data: %s", err, out.String())
	}

	return nil
}
