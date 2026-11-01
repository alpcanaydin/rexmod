# Changelog

All notable changes to rexmod. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- Rex's own "Check for Updates…" now runs rexmod's `update.sh` (official build, checked, then patched) and reports the result, instead of starting Sparkle and ending in "Update failed".
- Images in the editor pane stayed black: Neovim 0.12 runs its screen in a separate process, and the image plugin's `nvim_ui_send` writes never reached Rex. In Rex they now go straight to the pane's terminal.

## [0.1.4] - 2026-11-01

### Fixed

- The review comment box no longer pops up code completion while you type a note.
- Rex updates: the patched Rex showed "Update failed" because Sparkle's installer only accepts an app signed by Superlogical. Sparkle's checks are now off while Rex is patched (and back on after `unpatch.sh`), and the re-patch agent installs new Rex versions itself every hour with `update.sh`.

## [0.1.3] - 2026-11-01

### Fixed

- Codex: turns and comments go to the right pane. Codex runs hooks in a shared background server whose environment names whichever pane started it; rexmod now finds the pane where Codex works in that folder.
- OpenCode 2: the plugins load again (OpenCode 2 wants `{ id, setup }`), comments join your message through its `prompt` hook, a turn ends on `session.execution.*`, and the TUI half is listed in `cli.json`.

## [0.1.2] - 2026-11-01

### Fixed

- The installer installs Node when it's missing: without it, language servers installed with npm (TypeScript, JSON, YAML, CSS, Pyright…) couldn't be set up on a fresh Mac.

## [0.1.1] - 2026-11-01

### Added

- Homebrew: `brew install alpcanaydin/rexmod/rexmod`, then `rexmod-setup`.

### Fixed

- Only one patch runs at a time. After Rex updates itself, a re-patch agent and an install (or two agents) could swap the app bundle at once.

### Changed

- The README tells rexmod as what it is, a harness around your agent, and plays the demo video in place.

## [0.1.0] - 2026-11-01

First release.

### Added

- Changes panel on the right of the Rex window: Last Turn, Uncommitted and Branch change sets, and a Files tab, styled like Rex's own sidebar. `⌘⇧E` shows or hides it; the toggle sits with the pane actions.
- Editor pane: click a file and it opens in Neovim next to your agent, at the changed line; one editor per tab.
- Review comments: `<Space>r` on a line or selection in the editor; they reach the agent with your next message, or right away from the panel.
- `⌘P` file picker over the agent's repository, with the editor's syntax highlighting in the preview.
- Works with every agent Rex supports: Claude Code and Codex (hooks), OpenCode (plugin), Pi (extension). With several agents in a tab the panel follows the one you focused last.
- Language support on demand in the editor: open a file and its tree-sitter parser and language server are installed and started.
- Installer for a clean Mac, an uninstaller, and a LaunchAgent that re-applies the patch after Rex updates itself.
