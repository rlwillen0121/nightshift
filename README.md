# NIGHTSHIFT

Offline tactical-operations console. It is a local terminal simulation: five scripted phases, no network, no mission file, and no effect on a real system.

<p align="center">
  <img src="docs/nightshift-demo.gif" alt="NIGHTSHIFT terminal demo showing live telemetry, a signal capture, and an animated global traceroute" width="1200">
</p>

Released under the [MIT License](LICENSE).

The mission story is an append-only transcript, with a short header printed once and completed output left in terminal scrollback. While active, the telemetry panel and traceroute map redraw only their own fixed-width rows in place; the alternate screen is not used and older story lines are not repainted.

```sh
# Install the command.
go install github.com/rlwillen0121/nightshift/cmd/nightshift@latest
nightshift

# Or run it from a checkout.
go run ./cmd/nightshift
go run ./cmd/nightshift --seed 42 --ascii --no-color --reduced-motion
go run ./cmd/nightshift --help
```

`--seed` is an unsigned integer. If omitted, the seed comes from the local clock and is shown in the header. The stored transcript is plain ASCII, so the same seed matches whatever the flags. Glyphs and color are applied only as lines are written: rounded frames, a `callsign@nightshift ❯` prompt, `✓`/`▲` status marks, gradient loaders, seeded hex dumps, and `▰▱` meters. `--ascii` writes ASCII-only artwork. Color is on unless `--no-color`, `NO_COLOR`, or `TERM=dumb`. `TERM=dumb` also selects ASCII and reduced motion. Loaders fill, commands type out, banner titles decode in place, and the phase 02 waveform settles into a beacon pattern. On wide terminals with timed input polling, a live telemetry panel repaints at the end of the transcript while the shift is active; narrower terminals and terminals without polling support show it once as a static panel. CPU and traffic samples are fictional and seed-driven. Threat level appears below each phase banner, and occasional radio messages add fictional analyst chatter.

A printable key or a paste appends one canned burst for the active phase. A burst is four to thirteen lines of realistic console output, set off by a blank line: port scans, packet captures, auth logs, DNS answers, traceroutes, process lists, firewall rules, checksum runs, certificate probes, memory dumps, and sealed reports. Traceroutes include a fictional ASCII world map that traces city hops such as IAD, FRA, and SIN with made-up latency. Each phase draws from its own pool, and the seed, the trigger count, and the phase decide every choice, so the same seed always prints the same sequence. Two bursts in a row never use the same command. Keystrokes are not echoed or stored. Rare status-line glitches resolve back to their original text. Use `--bell` to ring the terminal bell when a `[warn]` line appears; it is off by default.

Enter before any burst in a phase prints that phase's hint and does not advance. After at least one burst, Enter prints a phase banner and advances. After the last phase, Enter prints a mission-complete debrief card with command, host, alert, and shift-time counts plus a mock handoff grade. Another Enter appends a restart banner and continues the same seed without clearing scrollback. Ignored controls do nothing. Ctrl+C is the only exit.

Hosts are `.invalid`, addresses are documentation ranges, and finding IDs are invented. Invalid arguments exit 2 with a sanitized message. A non-terminal stdin or stdout exits with `interactive terminal required`.
