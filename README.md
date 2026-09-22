# NIGHTSHIFT

Offline tactical-operations console. It is a local terminal simulation: five scripted phases, no network, no mission file, and no effect on a real system.

The live session is an append-only transcript. A short header is printed once. Each later action writes only the new lines, so the terminal's own scrollback can move back through older text. The alternate screen is not used, and earlier lines are not repainted.

```sh
go run ./cmd/nightshift
go run ./cmd/nightshift --seed 42 --ascii --no-color --reduced-motion
```

`--seed` is an unsigned integer. If omitted, the seed comes from the local clock and is shown in the header. The stored transcript is plain ASCII, so the same seed matches with or without color. Color is on unless `--no-color`, `NO_COLOR`, or `TERM=dumb`. `TERM=dumb` also selects ASCII and reduced motion. Nothing in the transcript animates, so a tick does not print or advance the scene.

A printable key or a paste appends one canned burst for the active phase. A burst is several original fictional lines, chosen from a fixed corpus by the seed and the trigger count, so the same seed always prints the same sequence. Further keys in that phase append later bursts and do not repeat the previous one. Keystrokes are not echoed or stored.

Enter before any burst in a phase prints that phase's hint and does not advance. After at least one burst, Enter prints a phase banner and advances. After the last phase, Enter prints the finale. Another Enter appends a restart banner and continues the same seed without clearing scrollback. Ignored controls do nothing. Ctrl+C is the only exit.

Hosts are `.invalid`, addresses are documentation ranges, and finding IDs are invented. Invalid arguments exit 2 with a sanitized message. A non-terminal stdin or stdout exits with `interactive terminal required`.
