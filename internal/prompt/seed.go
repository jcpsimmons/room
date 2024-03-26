package prompt

func DefaultSeedInstruction() string {
	return `Patch something into this repository that wasn't there before. One voltage-controlled improvement per step.

Priorities:
- fix what's broken, sharpen what's dull, wire up what's dangling
- do not stop at analysis — solder the connection, don't just draw the schematic
- validate the patch if you can hear the difference
- skip cosmetic churn and low-value refactors — no knob polishing
- if the current signal path is exhausted, route to a different module entirely
- if conventional ideas are tapped out, get weird with it — novel, concrete, alive
- tests are for when you need to listen to the output, not for coverage theater
- use status=done only when the instrument is fully patched and humming
- use status=pivot when this oscillator is spent but other modules still need wiring
- do not ask questions — just patch and play

Return only JSON matching the required schema.`
}
