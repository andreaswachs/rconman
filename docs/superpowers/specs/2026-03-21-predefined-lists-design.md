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

During config loading, each `TemplateParam` with `type: list` is validated:
- The `List` field must be non-empty
- The referenced name must exist as a key in `config.Lists`
- A missing or mismatched list name is a fatal startup error with a clear message

This follows the same pattern as secret resolution validation.

## UI Rendering

For `type: list` params, the templ template renders a native HTML `<datalist>` paired with a text input:

```html
<input
  type="text"
  id="param_pokemon"
  list="list_pokemon"
  autocomplete="off"
  class="input input-bordered input-sm w-full text-sm"
  placeholder="pokemon"
/>
<datalist id="list_pokemon">
  <option value="bulbasaur"/>
  <option value="charmander"/>
  <option value="squirtle"/>
</datalist>
```

List options are rendered server-side by the templ template. The `Lists map[string][]string` is passed into the `ServerPage` view alongside the server config so no extra API call is needed.

## Strict Validation (Client-Side)

Since `<datalist>` does not natively prevent arbitrary input, the existing send button JS handler is extended to validate list-type inputs before submission:

```js
const listId = input.getAttribute('list');
if (listId) {
  const allowed = [...document.querySelectorAll(`#${listId} option`)]
    .map(o => o.value);
  if (!allowed.includes(input.value)) {
    // apply DaisyUI error state, block submission
  }
}
```

## Data Flow

```
config.yaml
  └── Config.Lists (map[string][]string)
        └── passed to ServerPage templ component
              └── rendered as <datalist> elements in page HTML
                    └── referenced by <input list="..."> on list-type params
                          └── JS validates value is in allowed set before submit
```

## Backend Impact

None. The backend `/api/commands/{id}` handler receives the fully assembled command string as today. List validation is entirely a UI concern — no handler changes, no new routes, no schema changes.

## Files to Change

| File | Change |
|---|---|
| `internal/config/config.go` | Add `Lists` to `Config`; add `List` field to `TemplateParam`; add startup validation |
| `internal/views/pages.templ` | Pass `Lists` into `ServerPage`; render `<datalist>` for `type: list` params; extend JS validator |
| `config.example.yaml` | Add `lists` section example |
| `docs/configuration.md` | Document the new `lists` config key and `type: list` param type |
