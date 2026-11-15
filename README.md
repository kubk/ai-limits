# AI Limits

A macOS menu bar app that shows your Codex and Claude usage limits in a popover.

Limits are read through the official `codex` and `claude` CLIs (the same way [T3 Code](https://github.com/pingdotgg/t3code) does), only when you open the popover. No tokens are read and no prompts are sent.

Built with [MyGo](https://mygo.run).

```sh
go tool mygo dev     # run
go tool mygo build   # build/darwin-arm64/AI Limits.app
```
