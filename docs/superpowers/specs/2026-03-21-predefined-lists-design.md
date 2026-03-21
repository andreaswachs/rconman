# Predefined Lists for Command Parameters

**Date:** 2026-03-21
**Status:** Approved

## Overview

Add support for globally-defined named lists in the rconman configuration. Command template parameters can reference a named list to render a strict autocomplete input instead of a free-text field. Users can only submit values that appear in the list.

## Config Schema

A new top-level `lists` key in `config.yaml` maps list names to arrays of string values:

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

**List name constraints:** List names must be valid CSS identifiers (alphanumeric, hyphens, underscores — no spaces or special characters). Enforced at startup via `Validate()` using the pattern `^[a-zA-Z0-9_-]+$`, because list names appear in HTML `id` attributes.

In Go, this adds a `Lists` field to the top-level `Config` struct:

```go
type Config struct {
    Server    ServerConfig
    Log       LogConfig
    Store     StoreConfig
    Auth      AuthConfig
    Minecraft MinecraftConfig
    Lists     map[string][]string `yaml:"lists"`
}
```

## Parameter Type: `list`

A new `type: list` param references a global list by name:

```yaml
params:
  - name: pokemon
    type: list
    list: pokemon   # must match a key in the top-level lists map
```

The `TemplateParam` struct gains a `List` field:

```go
type TemplateParam struct {
    Name    string      `yaml:"name"`
    Type    string      `yaml:"type"`    // text, number, select, boolean, player, list
    Options []string    `yaml:"options"` // for select type
    List    string      `yaml:"list"`    // for list type: references config.Lists key
    Default interface{} `yaml:"default"`
    Min     int         `yaml:"min"`
    Max     int         `yaml:"max"`
}
```

## Validation at Startup

All new validation belongs in the existing `Validate()` function in `internal/config/config.go`.

Rules enforced:

1. **List name format:** Every key in `config.Lists` must match `^[a-zA-Z0-9_-]+$`. Fatal error otherwise.
2. **Non-empty lists:** A list with zero entries is a fatal error. An empty list would silently reject every user input with no useful error message.
3. **List reference exists:** For each `TemplateParam` with `type: list`, the `List` field must be non-empty and must match a key in `config.Lists`. Fatal error with a message identifying the server, category, command, and param name.

## UI Rendering

For `type: list` params, the templ template renders a native HTML `<datalist>` paired with a text input.

The template loop structure in `pages.templ` is two levels deep: an outer loop over categories (index `catIdx`) and an inner loop over `category.Templates` (index `tplIdx`). The inner index resets to 0 per category, so ids must include both to be unique. The id scheme is:

- Input: `param_{catIdx}_{tplIdx}_{param.Name}`
- Datalist: `list_{catIdx}_{tplIdx}_{param.Name}`

Example rendered output:

```html
<input
  type="text"
  id="param_0_1_pokemon"
  list="list_0_1_pokemon"
  autocomplete="off"
  class="input input-bordered input-sm w-full text-sm"
  placeholder="pokemon"
/>
<datalist id="list_0_1_pokemon">
  <option value="bulbasaur"/>
  <option value="charmander"/>
  <option value="squirtle"/>
</datalist>
```

**All param types** (not just `list`) use the scoped id scheme. This is required because the `executeCommand` JS uses a single regex to extract param names from ids — if list-type params use scoped ids but other types use the old `param_{name}` ids, the regex would break substitution for non-list params on the same card. Scoping universally keeps the extraction logic consistent and also fixes the pre-existing id collision for same-named params across template cards.

The inner loop in `pages.templ` must be updated from `for _, template := range category.Templates` to `for j, template := range category.Templates` to expose the template index.

## ServerPage Signature

The `Lists` map is passed into the `ServerPage` templ component. The updated signature is:

```go
templ ServerPage(session *auth.Session, server config.ServerDef, lists map[string][]string)
```

The call site in `internal/server/server.go` (currently `views.ServerPage(session, *server)`) becomes:

```go
views.ServerPage(session, *server, cfg.Lists).Render(r.Context(), w)
```

`cfg` is already in scope at that call site.

## Strict Validation (Client-Side)

The existing `executeCommand` script in `pages.templ` (lines 153–201) is updated as follows. The full replacement of the function body:

```js
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
        // Strip scoped prefix: param_{catIdx}_{tplIdx}_{name} → name
        const paramName = input.id.replace(/^param_\d+_\d+_/, "");
        const paramValue = input.value || "";

        // Strict list validation: if input is bound to a datalist, value must be in it
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
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: command })
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

Key changes vs. the original:
- `input-error` classes cleared at the top of each invocation so corrected fields don't retain error styling
- `hasError` flag initialized before the `forEach` loop
- Param name extraction updated to strip the two-part numeric prefix: `/^param_\d+_\d+_/`
- List validation block added inside the `forEach`, before command substitution
- Early return after the loop if `hasError` is true, re-enabling the button

## Data Flow

```
config.yaml
  └── Config.Lists (map[string][]string)
        └── Validate() — name format, non-empty, reference integrity
        └── passed to ServerPage(session, server, lists)
              └── rendered as <datalist id="list_{catIdx}_{tplIdx}_{name}">
                    └── referenced by <input list="list_{catIdx}_{tplIdx}_{name}">
                          └── JS clears errors → collects params → validates lists → fetch
```

## Backend Impact

None. The backend `/api/commands/{id}` handler receives the fully assembled command string as today. List validation is entirely a UI concern — no handler changes, no new routes, no schema changes.

## Files to Change

| File | Change |
|---|---|
| `internal/config/config.go` | Add `Lists` to `Config`; add `List` field to `TemplateParam`; add list validation to `Validate()` |
| `internal/views/pages.templ` | Update `ServerPage` signature to accept `lists`; render `<datalist>` for `type: list` params using `{catIdx}_{tplIdx}` scoped ids; replace `executeCommand` with the updated version above |
| `internal/server/server.go` | Pass `cfg.Lists` to `ServerPage` at the call site |
| `config.example.yaml` | Add a `lists` section at the top level with an example list and a corresponding `type: list` param in the commands section. The existing example uses incorrect type names (`string`, `enum`, `int`) that don't match the actual types (`text`, `select`, `number`) — this is a pre-existing inconsistency; the new additions will use correct names, and cleaning up the old entries is tracked separately. |
| `docs/configuration.md` | Add `lists` top-level key documentation; add `type: list` to the param types table with a note that the referenced list must exist in `lists`; document the list name character constraint |
