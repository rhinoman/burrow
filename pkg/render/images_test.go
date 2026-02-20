package render

import "testing"

func TestDetectImageTierText(t *testing.T) {
	tier := DetectImageTier("text")
	if tier != TierNone {
		t.Errorf("expected TierNone for 'text', got %d", tier)
	}
}

func TestDetectImageTierExternal(t *testing.T) {
	tier := DetectImageTier("external")
	if tier != TierNone {
		t.Errorf("expected TierNone for 'external', got %d", tier)
	}
}

func TestDetectImageTierUnknown(t *testing.T) {
	tier := DetectImageTier("garbage")
	if tier != TierNone {
		t.Errorf("expected TierNone for unknown value, got %d", tier)
	}
}

func TestDetectImageTierCaseInsensitive(t *testing.T) {
	tier := DetectImageTier("TEXT")
	if tier != TierNone {
		t.Errorf("expected TierNone for 'TEXT', got %d", tier)
	}
}

func TestDetectImageTierAuto(t *testing.T) {
	// In CI/test environments there's no terminal, so auto should return TierNone.
	tier := DetectImageTier("auto")
	if tier != TierNone {
		t.Logf("auto detected tier %d (expected TierNone in test env, but terminal may be present)", tier)
	}
}

func TestDetectImageTierEmpty(t *testing.T) {
	// Empty string should behave like "auto"
	tier := DetectImageTier("")
	if tier != TierNone {
		t.Logf("empty detected tier %d (expected TierNone in test env)", tier)
	}
}

func TestWriteInlineImageNone(t *testing.T) {
	// Writing with TierNone should be a no-op
	err := WriteInlineImage(nil, []byte("not real png"), TierNone)
	if err != nil {
		t.Errorf("expected no error for TierNone, got: %v", err)
	}
}
