package ui

import "testing"

func TestAccentBadgeUsesDarkForegroundOnBrightBackground(t *testing.T) {
	got := accentBadge(accentGold).GetForeground()

	if got == nil {
		t.Fatal("expected badge foreground to be set")
	}
	if got != bgColor {
		t.Fatalf("foreground = %#v, want %#v", got, bgColor)
	}
}

func TestAccentBadgeUsesLightForegroundOnDarkBackground(t *testing.T) {
	got := accentBadge(panelColor).GetForeground()

	if got == nil {
		t.Fatal("expected badge foreground to be set")
	}
	if got != textColor {
		t.Fatalf("foreground = %#v, want %#v", got, textColor)
	}
}
