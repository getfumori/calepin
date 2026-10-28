package model

import "fmt"

// CoreFiles returns the Vault Manifest and Core Model written by bootstrap.
func CoreFiles(id, name, timezone, created string) map[string]string {
	return map[string]string{
		ManifestPath: fmt.Sprintf(`---
_id: %s
_schema: fumori.vault
_version: 1
_created: %q
name: %q
timezone: %s
---

# Vault
`, id, created, name, timezone),
		modelDir + "/lifecycle.md": `---
_schema: fumori.model.lifecycle
_version: 1
states: [captured, organized, archived]
creation_state: captured
archived_state: archived
---

# Knowledge lifecycle
`,
		modelDir + "/types/note.md": `---
_schema: fumori.model.type
_version: 1
key: note
name: Note
default_state: captured
---

# Note
`,
		modelDir + "/types/daily-note.md": `---
_schema: fumori.model.type
_version: 1
key: daily-note
name: Daily Note
default_state: organized
---

# Daily Note
`,
		modelDir + "/types/task-list.md": `---
_schema: fumori.model.type
_version: 1
key: task-list
name: Task List
default_state: organized
---

# Task List
`,
		modelDir + "/views/inbox.md": `---
_schema: fumori.model.view
_version: 1
key: inbox
name: Inbox
query:
  filter:
    all:
      - {field: kind, operator: equals, value: standalone}
      - {field: state, operator: equals, value: captured}
  layout: list
---

# Inbox
`,
	}
}
