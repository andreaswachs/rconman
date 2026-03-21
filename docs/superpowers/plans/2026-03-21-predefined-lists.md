# Predefined Lists for Command Parameters — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add global named lists to rconman config that command params can reference, rendering as a strict autocomplete input (HTML datalist).

**Architecture:** A top-level `lists` key in `config.yaml` holds named string arrays. A new `type: list` param type references a list by name. The templ `ServerPage` component receives the lists map and renders `<datalist>` elements server-side. Client-side JS validates that submitted values are in the allowed list before sending the fetch. No backend changes needed.

**Tech Stack:** Go 1.22+, templ (go generate), Tailwind CSS + DaisyUI, vanilla JS inline in templ scripts. Test runner: `go test ./...`.

---

## File Map

| File | Change |
|---|---|
| `internal/config/config.go` | Add `Lists map[string][]string` to `Config`; add `List string` to `TemplateParam`; add three validation rules to `Validate()` |
| `internal/config/config_test.go` | Add tests for the three new validation rules |
| `internal/views/pages.templ` | Update `ServerPage` signature; scope all param ids to `{catIdx}_{tplIdx}_{name}`; render `<datalist>` for `type: list`; replace `executeCommand` script |
| `internal/views/pages_templ.go` | Auto-generated — regenerate with `go generate ./...` after templ changes |
| `internal/server/server.go` | Pass `cfg.Lists` to `ServerPage` at the call site (line 105) |
| `config.example.yaml` | Add top-level `lists` section with example; add `type: list` param example |
| `docs/configuration.md` | Document `lists` key; add `list` param type to the parameter types reference |

---

## Task 1: Config struct changes and validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

### Background

The `Config` struct (line 12) needs a `Lists` field. The `TemplateParam` struct (line 104) needs a `List` field. The `Validate()` function (line 133) needs three new checks:

1. Every key in `config.Lists` must match `^[a-zA-Z0-9_-]+$`
2. Every list in `config.Lists` must have at least one entry
3. Every `TemplateParam` with `type: list` must have a `List` value that exists as a key in `config.Lists`

- [ ] **Step 1: Write the failing tests**

Add these tests to `internal/config/config_test.go`. They all go at the bottom of the file, after the existing tests.

```go
func TestValidateConfig_ListNameInvalidChars(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{
			"my list": {"a", "b"}, // space is invalid
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for list name with space")
	}
}

func TestValidateConfig_ListEmpty(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{
			"pokemon": {}, // empty list
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestValidateConfig_ListParamReferenceMissing(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{},
		Minecraft: MinecraftConfig{
			Servers: []ServerDef{
				{
					ID: "test",
					RCON: RCONConfig{
						Host:     "localhost",
						Port:     25575,
						Password: &SecretValue{Value: "pass"},
					},
					Commands: []CommandCategory{
						{
							Category: "Test",
							Templates: []CommandTemplate{
								{
									Name:    "cmd",
									Command: "/cmd {x}",
									Params: []TemplateParam{
										{Name: "x", Type: "list", List: "missing-list"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for list param referencing missing list")
	}
}

func TestValidateConfig_ListParamValid(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{
			"pokemon": {"bulbasaur", "charmander", "squirtle"},
		},
		Minecraft: MinecraftConfig{
			Servers: []ServerDef{
				{
					ID: "test",
					RCON: RCONConfig{
						Host:     "localhost",
						Port:     25575,
						Password: &SecretValue{Value: "pass"},
					},
					Commands: []CommandCategory{
						{
							Category: "Test",
							Templates: []CommandTemplate{
								{
									Name:    "cmd",
									Command: "/cmd {x}",
									Params: []TemplateParam{
										{Name: "x", Type: "list", List: "pokemon"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error for valid list param, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -run "TestValidateConfig_List" -v
```

Expected: all four tests FAIL — the validation logic doesn't exist yet.

- [ ] **Step 3: Add `Lists` to `Config` and `List` to `TemplateParam`**

In `internal/config/config.go`, make these two changes:

**Change 1** — `Config` struct (line 12), add `Lists` after `Minecraft`:

```go
// Config represents the complete application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Log        LogConfig        `yaml:"log"`
	Store      StoreConfig      `yaml:"store"`
	Auth       AuthConfig       `yaml:"auth"`
	Minecraft  MinecraftConfig  `yaml:"minecraft"`
	Lists      map[string][]string `yaml:"lists"`
}
```

**Change 2** — `TemplateParam` struct (line 103), add `List` after `Options`:

```go
// TemplateParam represents a parameter in a command template
type TemplateParam struct {
	Name    string        `yaml:"name"`
	Type    string        `yaml:"type"`
	Options []string      `yaml:"options"`
	List    string        `yaml:"list"`
	Default interface{}   `yaml:"default"`
	Min     int           `yaml:"min"`
	Max     int           `yaml:"max"`
}
```

- [ ] **Step 4: Add list validation to `Validate()`**

The import `regexp` is needed. Add it to the import block at the top of `config.go` (alongside `fmt`, `os`, `time`).

In `Validate()`, add this block **after** the existing Minecraft server password loop (after line 164, before `return nil`):

```go
	// Validate global lists
	listNameRe := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	for name, entries := range c.Lists {
		if !listNameRe.MatchString(name) {
			return fmt.Errorf("list name %q is invalid: must contain only alphanumeric characters, hyphens, and underscores", name)
		}
		if len(entries) == 0 {
			return fmt.Errorf("list %q must have at least one entry", name)
		}
	}

	// Validate list-type params reference an existing list
	for _, server := range c.Minecraft.Servers {
		for _, category := range server.Commands {
			for _, tmpl := range category.Templates {
				for _, param := range tmpl.Params {
					if param.Type == "list" {
						if param.List == "" {
							return fmt.Errorf("server %q category %q template %q param %q: type=list requires a 'list' field", server.ID, category.Category, tmpl.Name, param.Name)
						}
						if _, ok := c.Lists[param.List]; !ok {
							return fmt.Errorf("server %q category %q template %q param %q: references undefined list %q", server.ID, category.Category, tmpl.Name, param.Name, param.List)
						}
					}
				}
			}
		}
	}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/config/... -run "TestValidateConfig_List" -v
```

Expected: all four tests PASS.

- [ ] **Step 6: Run full test suite to check for regressions**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add global lists and list param type with startup validation"
```

---

## Task 2: Update ServerPage signature and call site

**Files:**
- Modify: `internal/views/pages.templ`
- Modify: `internal/server/server.go`

### Background

`ServerPage` currently takes `(session *auth.Session, server config.ServerDef)`. It needs to also receive `lists map[string][]string` so the template can render datalist options. The call site in `server.go` line 105 must pass `cfg.Lists`.

After editing `pages.templ`, run `go generate ./...` to regenerate `pages_templ.go`. This is required before `go test` will compile.

- [ ] **Step 1: Update `ServerPage` signature in `pages.templ`**

In `internal/views/pages.templ`, line 57, change:

```templ
templ ServerPage(session *auth.Session, server config.ServerDef) {
```

to:

```templ
templ ServerPage(session *auth.Session, server config.ServerDef, lists map[string][]string) {
```

- [ ] **Step 2: Update call site in `server.go`**

In `internal/server/server.go`, line 105, change:

```go
			views.ServerPage(session, *server).Render(r.Context(), w)
```

to:

```go
			views.ServerPage(session, *server, cfg.Lists).Render(r.Context(), w)
```

- [ ] **Step 3: Regenerate templ files**

```bash
go generate ./...
```

Expected: no errors. This regenerates `internal/views/pages_templ.go`.

- [ ] **Step 4: Verify compilation**

```bash
go build ./...
```

Expected: compiles without errors.

- [ ] **Step 5: Run tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/views/pages.templ internal/views/pages_templ.go internal/server/server.go
git commit -m "feat(views): pass lists map to ServerPage"
```

---

## Task 3: Render list-type params with datalist + scope all param ids

**Files:**
- Modify: `internal/views/pages.templ`

### Background

Two changes are needed in the param rendering loop:

1. **Scope all param ids** — the current `id={ "param_" + param.Name }` collides when the same param name appears across multiple template cards. All param ids become `fmt.Sprintf("param_%d_%d_%s", i, j, param.Name)` where `i` is the category index (already available as `i`) and `j` is the template index (needs to be added to the inner loop).

2. **Render `<datalist>` for `type: list`** — a new branch in the param type switch renders `<input list="...">` paired with a `<datalist>` populated from `lists[param.List]`.

The `fmt` package is already imported in `pages.templ`.

- [ ] **Step 1: Update the inner template loop to capture index**

In `internal/views/pages.templ`, line 109, change:

```templ
									for _, template := range category.Templates {
```

to:

```templ
									for j, template := range category.Templates {
```

- [ ] **Step 2: Scope all param ids and add datalist rendering**

Replace the entire param rendering block (lines 114–133 in the original file, the `if len(template.Params) > 0` block) with:

```templ
												if len(template.Params) > 0 {
													<div class="space-y-3 my-4">
														for _, param := range template.Params {
															<div>
																<label class="label">
																	<span class="label-text text-xs text-gray-300">{ param.Name }</span>
																</label>
																if param.Type == "select" && len(param.Options) > 0 {
																	<select class="select select-bordered select-sm w-full text-sm" id={ fmt.Sprintf("param_%d_%d_%s", i, j, param.Name) }>
																		for _, opt := range param.Options {
																			<option value={ opt }>{ opt }</option>
																		}
																	</select>
																} else if param.Type == "list" {
																	<input
																		type="text"
																		id={ fmt.Sprintf("param_%d_%d_%s", i, j, param.Name) }
																		list={ fmt.Sprintf("list_%d_%d_%s", i, j, param.Name) }
																		autocomplete="off"
																		placeholder={ param.Name }
																		class="input input-bordered input-sm w-full text-sm"
																	/>
																	<datalist id={ fmt.Sprintf("list_%d_%d_%s", i, j, param.Name) }>
																		for _, opt := range lists[param.List] {
																			<option value={ opt }></option>
																		}
																	</datalist>
																} else {
																	<input type="text" placeholder={ param.Name } class="input input-bordered input-sm w-full text-sm" id={ fmt.Sprintf("param_%d_%d_%s", i, j, param.Name) }/>
																}
															</div>
														}
													</div>
												}
```

- [ ] **Step 3: Replace the `executeCommand` script**

Replace the entire `script executeCommand(...)` block (lines 153–201) with the following. This replaces the old param name extraction regex, adds error clearing, list validation, and an early-return guard:

```templ
script executeCommand(serverId string, commandTemplate string, commandName string) {
	const btn = event.target;
	btn.disabled = true;
	btn.textContent = "Executing...";

	// Clear previous error states
	btn.closest(".card").querySelectorAll(".input-error").forEach(el => {
		el.classList.remove("input-error");
	});

	// Collect parameter values and validate list-type inputs
	let command = commandTemplate;
	let hasError = false;
	const paramInputs = btn.closest(".card").querySelectorAll("[id^='param_']");
	paramInputs.forEach(input => {
		// Strip scoped prefix param_{catIdx}_{tplIdx}_{name} → name
		const paramName = input.id.replace(/^param_\d+_\d+_/, "");
		const paramValue = input.value || "";

		// Strict list validation: value must be one of the datalist options
		const listId = input.getAttribute("list");
		if (listId) {
			const allowed = [...document.querySelectorAll("#" + listId + " option")]
				.map(o => o.value);
			if (!allowed.includes(paramValue)) {
				input.classList.add("input-error");
				hasError = true;
				return;
			}
		}

		command = command.replace("{" + paramName + "}", paramValue);
	});

	if (hasError) {
		btn.disabled = false;
		btn.textContent = "Execute";
		return;
	}

	fetch("/api/commands/" + serverId, {
		method: "POST",
		headers: {
			"Content-Type": "application/json"
		},
		body: JSON.stringify({
			command: command
		})
	})
	.then(r => {
		if (!r.ok) return r.text().then(text => { throw new Error(text); });
		return r.json();
	})
	.then(data => {
		if (data.status === "executed") {
			alert("✓ Command executed!\n\n" + (data.response || "Success"));
			btn.textContent = "Execute";
		} else if (data.error) {
			alert("✗ Error: " + data.error);
			btn.textContent = "Execute";
		} else {
			alert("✓ Command executed!");
			btn.textContent = "Execute";
		}
	})
	.catch(err => {
		alert("✗ Failed to execute: " + err.message);
		btn.textContent = "Execute";
		console.error(err);
	})
	.finally(() => {
		btn.disabled = false;
	});
}
```

- [ ] **Step 4: Regenerate templ files**

```bash
go generate ./...
```

Expected: no errors.

- [ ] **Step 5: Verify compilation**

```bash
go build ./...
```

Expected: compiles without errors.

- [ ] **Step 6: Run tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/views/pages.templ internal/views/pages_templ.go
git commit -m "feat(views): render list params as datalist autocomplete, scope all param ids"
```

---

## Task 4: Update config.example.yaml and docs/configuration.md

**Files:**
- Modify: `config.example.yaml`
- Modify: `docs/configuration.md`

### Background

Operators need to know how to configure lists. The example file and reference docs both need updating.

Note: `config.example.yaml` contains pre-existing stale type names (`type: "string"`, `type: "enum"`, `type: "int"`) that don't match the real types. The new example will use the correct `type: list` name. Cleaning up the stale entries is a separate follow-up and not part of this task.

- [ ] **Step 1: Add `lists` section to `config.example.yaml`**

Add the following block at the top level of `config.example.yaml`, immediately after the `minecraft:` section ends (after the last line of the second server's `commands:` block, around line 213) and before the `# Configuration notes:` comment:

```yaml
# Global predefined lists
# Define named lists of values here. Command template params with type: list
# can reference these by name, rendering as a strict autocomplete input.
# List names must be alphanumeric, hyphens, and underscores only (no spaces).
lists:
  # Example: a list of Pokemon names for a command that takes a pokemon argument
  # Usage in params: { name: pokemon, type: list, list: pokemon }
  pokemon:
    - bulbasaur
    - charmander
    - squirtle
    - pikachu
    - mewtwo
```

Also add a `type: list` example to one of the command templates in the second server's commands section. After the `- name: "Save"` template, add:

```yaml
            - name: "Catch Pokemon"
              description: "Run a pokemon command"
              command: "pokemon catch {pokemon}"
              params:
                - name: pokemon
                  type: list
                  list: pokemon
```

- [ ] **Step 2: Add `lists` documentation to `docs/configuration.md`**

**2a.** In the Table of Contents (line 7), add a new entry under `minecraft`:

```markdown
  - [lists](#lists)
```

Place it after the `- [minecraft](#minecraft)` entry.

**2b.** Add a new top-level `### lists` section. Place it between the `### minecraft` section and `## Complete Example`. Insert:

```markdown
---

### `lists`

Defines globally-scoped named lists of string values. Lists are referenced by command template parameters with `type: list`, which render as strict autocomplete inputs — the user sees suggestions from the list and can only submit a value that appears in it.

| Field | Type | Required | Description |
|---|---|---|---|
| `lists.<name>` | list of strings | no | A named list. The name must contain only alphanumeric characters, hyphens (`-`), and underscores (`_`). Must have at least one entry. |

```yaml
lists:
  pokemon:
    - bulbasaur
    - charmander
    - squirtle
  gamemodes:
    - survival
    - creative
    - adventure
```

> **Startup validation:** All lists are validated at startup. An invalid list name (e.g. containing spaces), an empty list, or a `type: list` param that references a list name not defined here will cause rconman to exit with a clear error message.

---
```

**2c.** In the `### Template Parameter Types` section, add a new `#### list` entry after the existing `#### player` entry:

```markdown
#### `list`

A strict autocomplete input. The user types and sees suggestions from a named list defined in the top-level [`lists`](#lists) config key. Only values present in the list are accepted — submitting an unlisted value is blocked in the UI.

```yaml
- name: pokemon
  type: list
  list: pokemon    # must match a key defined under the top-level lists key
```

> **Cross-reference:** The value of `list` must exactly match a key in `lists`. rconman will not start if the referenced list does not exist.
```

- [ ] **Step 3: Verify the project still builds and tests pass**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add config.example.yaml docs/configuration.md
git commit -m "docs: add lists config section and list param type to example and reference docs"
```

---

## Verification Checklist

After all tasks complete:

- [ ] `go test ./...` passes with no failures
- [ ] `go build ./...` succeeds
- [ ] `go generate ./...` produces no errors
- [ ] Config with a `type: list` param referencing a valid list loads without error
- [ ] Config with a `type: list` param referencing a missing list fails at startup with a clear error message
- [ ] Config with an empty list fails at startup with a clear error message
- [ ] Config with a list name containing a space fails at startup with a clear error message
