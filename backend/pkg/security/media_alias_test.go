package security

import "testing"

func TestMediaOutputCodeIsReadableAndNormalizes(t *testing.T) {
	code, err := GenerateMediaOutputCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 19 || code[4] != '-' || code[9] != '-' || code[14] != '-' {
		t.Fatalf("unexpected display code %q", code)
	}
	compact, ok := NormalizeMediaOutputCode(code)
	if !ok || len(compact) != MediaOutputCodeSymbols || FormatMediaOutputCode(compact) != code {
		t.Fatalf("code did not round trip: code=%q compact=%q ok=%v", code, compact, ok)
	}
}

func TestMediaOutputCodeAcceptsCaseSeparatorsAndCrockfordAliases(t *testing.T) {
	compact, ok := NormalizeMediaOutputCode("abcd-efgh-jkmn-pqrs")
	if !ok || compact != "ABCDEFGHJKMNPQRS" {
		t.Fatalf("friendly code was rejected: compact=%q ok=%v", compact, ok)
	}
	compact, ok = NormalizeMediaOutputCode("abcd-efgh-jkmn-pqol")
	if !ok || compact != "ABCDEFGHJKMNPQ01" {
		t.Fatalf("Crockford aliases were not normalized: compact=%q ok=%v", compact, ok)
	}
	if _, ok := NormalizeMediaOutputCode("too-short"); ok {
		t.Fatal("invalid output code was accepted")
	}
}
