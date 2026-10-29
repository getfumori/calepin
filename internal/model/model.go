// Package model loads the Vault Manifest and the Organization Model from
// .calepin/ and defines the Core Model written by bootstrap.
package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/getfumori/fumori/internal/format"
)

const (
	Dir          = ".calepin"
	ManifestPath = ".calepin/vault.md"
	modelDir     = ".calepin/model"
)

// Manifest is the Vault's identity and portable configuration.
type Manifest struct {
	ID       string
	Name     string
	Timezone *time.Location
}

type Property struct {
	Key      string   `json:"key" yaml:"key"`
	Name     string   `json:"name" yaml:"name"`
	Kind     string   `json:"kind" yaml:"kind"`
	Options  []string `json:"options,omitempty" yaml:"options,omitempty"`
	Default  any      `json:"default,omitempty" yaml:"default,omitempty"`
	Required bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Advisory string   `json:"advisory,omitempty" yaml:"advisory,omitempty"`
}

type Type struct {
	Key          string     `json:"key"`
	Name         string     `json:"name"`
	DefaultState string     `json:"defaultState,omitempty"`
	Properties   []Property `json:"properties"`
}

type Relationship struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Cardinality string   `json:"cardinality"`
	Inverse     string   `json:"inverse"`
	TargetTypes []string `json:"targetTypes"`
}

// Filter is a QuerySpec filter: a predicate or one boolean combinator.
type Filter struct {
	Field    string   `json:"field,omitempty" yaml:"field,omitempty"`
	Operator string   `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value    any      `json:"value,omitempty" yaml:"value,omitempty"`
	All      []Filter `json:"all,omitempty" yaml:"all,omitempty"`
	Any      []Filter `json:"any,omitempty" yaml:"any,omitempty"`
	Not      *Filter  `json:"not,omitempty" yaml:"not,omitempty"`
}

type Order struct {
	Field     string `json:"field" yaml:"field"`
	Direction string `json:"direction" yaml:"direction"`
}

type QuerySpec struct {
	Filter         *Filter  `json:"filter,omitempty" yaml:"filter,omitempty"`
	Order          []Order  `json:"order,omitempty" yaml:"order,omitempty"`
	GroupBy        string   `json:"groupBy,omitempty" yaml:"group_by,omitempty"`
	Layout         string   `json:"layout,omitempty" yaml:"layout,omitempty"`
	VisibleColumns []string `json:"visibleColumns,omitempty" yaml:"visible_columns,omitempty"`
}

type View struct {
	Key   string    `json:"key"`
	Name  string    `json:"name"`
	Query QuerySpec `json:"query"`
}

// Model is the loaded Organization Model.
type Model struct {
	States        []string       `json:"states"`
	CreationState string         `json:"standaloneCreationState"`
	ArchivedState string         `json:"archivedState"`
	Types         []Type         `json:"types"`
	Relationships []Relationship `json:"relationships"`
	Views         []View         `json:"views"`
}

func (m *Model) Type(key string) *Type {
	for i := range m.Types {
		if m.Types[i].Key == key {
			return &m.Types[i]
		}
	}
	return nil
}

func (m *Model) View(key string) *View {
	for i := range m.Views {
		if m.Views[i].Key == key {
			return &m.Views[i]
		}
	}
	return nil
}

func (m *Model) HasState(state string) bool { return slices.Contains(m.States, state) }

var schemaKey = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func readDoc(root, rel, schema string) (*format.Document, error) {
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	doc, err := format.Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	if got, _ := doc.String("_schema"); got != schema {
		return nil, fmt.Errorf("%s: _schema must be %s", rel, schema)
	}
	if got, _ := doc.String("_version"); got != "1" {
		return nil, fmt.Errorf("%s: unsupported _version", rel)
	}
	return doc, nil
}

func decode(doc *format.Document, rel string, fields map[string]any) error {
	for key, target := range fields {
		if _, err := doc.Decode(key, target); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// LoadManifest reads .calepin/vault.md.
func LoadManifest(root string) (*Manifest, error) {
	doc, err := readDoc(root, ManifestPath, "calepin.vault")
	if err != nil {
		return nil, err
	}
	id, _ := doc.String("_id")
	name, _ := doc.String("name")
	zone, _ := doc.String("timezone")
	if id == "" || name == "" {
		return nil, fmt.Errorf("%s: _id and name are required", ManifestPath)
	}
	location, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		return nil, fmt.Errorf("%s: timezone must be an IANA time zone", ManifestPath)
	}
	return &Manifest{ID: id, Name: name, Timezone: location}, nil
}

// Load reads and validates the Organization Model.
func Load(root string) (*Model, error) {
	lifecyclePath := modelDir + "/lifecycle.md"
	lifecycle, err := readDoc(root, lifecyclePath, "calepin.model.lifecycle")
	if err != nil {
		return nil, err
	}
	m := &Model{}
	if err := decode(lifecycle, lifecyclePath, map[string]any{
		"states": &m.States, "creation_state": &m.CreationState, "archived_state": &m.ArchivedState,
	}); err != nil {
		return nil, err
	}
	if len(m.States) == 0 || !m.HasState(m.CreationState) || !m.HasState(m.ArchivedState) {
		return nil, fmt.Errorf("%s: states must include creation_state and archived_state", lifecyclePath)
	}

	err = eachModelFile(root, "types", "calepin.model.type", func(rel, key string, doc *format.Document) error {
		t := Type{Key: key, Properties: []Property{}}
		if err := decode(doc, rel, map[string]any{"name": &t.Name, "default_state": &t.DefaultState, "properties": &t.Properties}); err != nil {
			return err
		}
		if t.DefaultState != "" && !m.HasState(t.DefaultState) {
			return fmt.Errorf("%s: unknown default_state %q", rel, t.DefaultState)
		}
		m.Types = append(m.Types, t)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, required := range []string{"note", "daily-note", "task-list"} {
		if m.Type(required) == nil {
			return nil, fmt.Errorf("%s/types/%s.md is required", modelDir, required)
		}
	}
	err = eachModelFile(root, "relationships", "calepin.model.relationship", func(rel, key string, doc *format.Document) error {
		r := Relationship{Key: key, TargetTypes: []string{}}
		if err := decode(doc, rel, map[string]any{"name": &r.Name, "cardinality": &r.Cardinality, "inverse": &r.Inverse, "target_types": &r.TargetTypes}); err != nil {
			return err
		}
		if r.Cardinality != "one" && r.Cardinality != "many" {
			return fmt.Errorf("%s: cardinality must be one or many", rel)
		}
		m.Relationships = append(m.Relationships, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = eachModelFile(root, "views", "calepin.model.view", func(rel, key string, doc *format.Document) error {
		v := View{Key: key}
		if err := decode(doc, rel, map[string]any{"name": &v.Name, "query": &v.Query}); err != nil {
			return err
		}
		m.Views = append(m.Views, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if m.Relationships == nil {
		m.Relationships = []Relationship{}
	}
	return m, nil
}

func eachModelFile(root, dir, schema string, fn func(rel, key string, doc *format.Document) error) error {
	entries, err := os.ReadDir(filepath.Join(root, modelDir, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		rel := modelDir + "/" + dir + "/" + name
		doc, err := readDoc(root, rel, schema)
		if err != nil {
			return err
		}
		key, _ := doc.String("key")
		if key != strings.TrimSuffix(name, ".md") || !schemaKey.MatchString(key) {
			return fmt.Errorf("%s: key must match the filename", rel)
		}
		if err := fn(rel, key, doc); err != nil {
			return err
		}
	}
	return nil
}
