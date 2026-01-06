# Typtel

Keystroke and mouse distance metrics for developers. Tracks every keypress including modifiers, escape sequences, and shortcuts.

## Installation

```sh
brew tap abaj8494/typing-telemetry
brew install --cask typtel
```

### Accessibility Permissions

Typtel requires accessibility permissions to capture input events:

1. Open **System Settings** > **Privacy & Security** > **Accessibility**
2. Click **+** and navigate to `/Applications/Typtel.app`
3. Enable the checkbox for Typtel
4. Restart the app from the menu bar or via `open /Applications/Typtel.app`

## CLI

The `typtel` command provides a terminal interface to your typing data.

```sh
typtel              # Interactive TUI dashboard
typtel today        # Today's keystroke count
typtel stats        # Detailed statistics
typtel test         # Typing speed test
typtel test -w 50   # Test with 50 words
```

### Typing Test

| Key      | Action             |
|----------|-------------------|
| `tab`    | Restart with new words |
| `esc`    | Options menu       |
| `enter`  | Start new test     |
| `ctrl+c` | Quit               |

Options include layout emulation, live WPM display, test length, uppercase, punctuation, and pace caret.

## Menu Bar

Click the menu bar icon to view:

- Daily and weekly keystroke/word/click counts
- Mouse distance traveled
- Charts and heatmaps
- Stillness leaderboard (least mouse movement)
- Settings

## Inertia

Inertia provides accelerating key repeat. When enabled, held keys repeat at increasing speeds based on an acceleration table derived from [accelerated-jk.nvim](https://github.com/rainbowhxch/accelerated-jk.nvim).

Toggle and configure via **Settings** > **Inertia Settings** in the menu bar:

| Setting           | Options                          |
|-------------------|----------------------------------|
| Enable/Disable    | Toggle inertia on or off         |
| Max Speed         | Ultra Fast (140/s), Very Fast (125/s), Fast (83/s), Medium (50/s), Slow (20/s) |
| Threshold         | 50ms - 300ms before acceleration |
| Acceleration Rate | 0.25x - 2.0x multiplier          |

Double-tap Shift to reset acceleration to base speed.

## Data Storage

All data is stored locally in `~/.local/share/typtel/`:

- `typtel.db` - SQLite database
- `logs/` - Application logs

No data is sent externally.

## Updating

```sh
brew update && brew upgrade --cask typtel
```

## Uninstalling

```sh
brew uninstall --cask typtel
rm -rf ~/.local/share/typtel  # Optional: remove data
```

## License

MIT
