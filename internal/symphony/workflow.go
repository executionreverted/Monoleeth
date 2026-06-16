package symphony

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

const defaultPromptTemplate = `You are working on a Linear issue.

Identifier: {{ issue.identifier }}
Title: {{ issue.title }}

Body:
{% if issue.description %}
{{ issue.description }}
{% else %}
No description provided.
{% endif %}
`

func LoadWorkflow(path string) (WorkflowDefinition, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	configYAML, prompt := splitFrontMatter(content)
	cfg := defaultConfig()
	if strings.TrimSpace(string(configYAML)) != "" {
		decoder := yaml.NewDecoder(bytes.NewReader(configYAML))
		decoder.KnownFields(false)
		if err := decoder.Decode(&cfg); err != nil {
			return WorkflowDefinition{}, err
		}
	}
	dir := filepath.Dir(abs)
	cfg, err = finalizeConfig(cfg, dir)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = defaultPromptTemplate
	}
	return WorkflowDefinition{
		Path:           abs,
		Dir:            dir,
		Config:         cfg,
		PromptTemplate: prompt,
		ModTime:        info.ModTime(),
	}, nil
}

func splitFrontMatter(content []byte) ([]byte, string) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, strings.TrimSpace(text)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
	}
	return []byte(strings.Join(lines[1:], "\n")), ""
}

func ReloadWorkflowIfChanged(current WorkflowDefinition) (WorkflowDefinition, bool, error) {
	info, err := os.Stat(current.Path)
	if err != nil {
		return current, false, err
	}
	if !info.ModTime().After(current.ModTime) {
		return current, false, nil
	}
	next, err := LoadWorkflow(current.Path)
	if err != nil {
		return current, false, err
	}
	return next, true, nil
}

func EnsureWorkflowPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("workflow path is a directory")
	}
	return nil
}
