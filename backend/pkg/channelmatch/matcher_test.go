package channelmatch

import "testing"

func TestNormalizeTvgID(t *testing.T) {
	got := NormalizeTvgID("  BBCNewsChannel.UK  ")
	if got != "bbcnewschannel.uk" {
		t.Fatalf("NormalizeTvgID() = %q", got)
	}
	if got := NormalizeTvgID("Straße.DE"); got != "strasse.de" {
		t.Fatalf("NormalizeTvgID() did not Unicode-case-fold: %q", got)
	}
}

func TestParseNameNormalizesSafePresentationDifferences(t *testing.T) {
	tests := map[string]string{
		"The History Channel FHD": "history channel",
		"RTL II HEVC":             "rtl 2",
		"Kabel Eins HD":           "kabel 1",
		"DAZN 01 UHD":             "dazn 1",
		"A+E Network":             "a plus e network",
	}

	for input, expected := range tests {
		if got := ParseName(input).Canonical; got != expected {
			t.Errorf("ParseName(%q).Canonical = %q, want %q", input, got, expected)
		}
	}
}

func TestParseNamePreservesAmbiguousBrandAndRegionTokens(t *testing.T) {
	if got := ParseName("V HD").Canonical; got != "v" {
		t.Fatalf("standalone Roman-looking brand normalized to %q", got)
	}
	parsed := ParseName("Spectrum News 1 SD")
	if parsed.Canonical != "spectrum news 1 sd" || len(parsed.Regions) != 1 || parsed.Regions[0] != "sd" {
		t.Fatalf("local SD suffix was not preserved: %+v", parsed)
	}
}

func TestRankPrefersExactCanonicalName(t *testing.T) {
	results := Rank("CNN FHD", []Candidate{
		{ID: 1, Name: "CNN HD"},
		{ID: 2, Name: "CNN International"},
	}, 5)

	if len(results) == 0 {
		t.Fatal("Rank() returned no results")
	}
	if results[0].Candidate.ID != 1 || results[0].Method != MethodExactName || results[0].Score != 1 {
		t.Fatalf("unexpected best result: %+v", results[0])
	}
}

func TestRankRejectsConflictingBrandNumbers(t *testing.T) {
	results := Rank("ESPN 2 HD", []Candidate{
		{ID: 1, Name: "ESPN 1"},
		{ID: 2, Name: "ESPN 2"},
	}, 5)

	if len(results) != 1 || results[0].Candidate.ID != 2 {
		t.Fatalf("Rank() returned conflicting-number candidates: %+v", results)
	}
}

func TestRankRejectsConflictingRegionsAndFeeds(t *testing.T) {
	tests := []struct {
		query     string
		candidate string
	}{
		{query: "Spectrum News 1 NC", candidate: "Spectrum News 1 NY"},
		{query: "HBO East", candidate: "HBO West"},
		{query: "US CNN", candidate: "UK CNN"},
	}

	for _, test := range tests {
		if results := Rank(test.query, []Candidate{{ID: 1, Name: test.candidate}}, 5); len(results) != 0 {
			t.Errorf("Rank(%q, %q) = %+v, want no match", test.query, test.candidate, results)
		}
	}
}

func TestRankDoesNotCollapseDifferentNetworks(t *testing.T) {
	tests := []struct {
		query     string
		candidate string
	}{
		{query: "US Antenna TV", candidate: "US Get TV"},
		{query: "US MTV Classic East", candidate: "US MTV East"},
		{query: "BBC News", candidate: "BBC World News"},
	}

	for _, test := range tests {
		results := Rank(test.query, []Candidate{{ID: 1, Name: test.candidate}}, 1)
		if len(results) > 0 && results[0].Score >= 0.96 {
			t.Errorf("Rank(%q, %q) produced unsafe score %.4f", test.query, test.candidate, results[0].Score)
		}
	}
}

func TestRankReportsRunnerUpScore(t *testing.T) {
	results := Rank("Sky Cinema Action", []Candidate{
		{ID: 1, Name: "Sky Cinema Action HD"},
		{ID: 2, Name: "Sky Cinema"},
		{ID: 3, Name: "Sky Action"},
	}, 3)

	if len(results) < 2 {
		t.Fatalf("Rank() returned %d results, want at least two", len(results))
	}
	if results[0].RunnerUpScore != results[1].Score {
		t.Fatalf("runner-up score = %.4f, want %.4f", results[0].RunnerUpScore, results[1].Score)
	}
}

func TestAutomaticRejectsAmbiguousCanonicalDuplicates(t *testing.T) {
	results := Rank("CNN HD", []Candidate{
		{ID: 1, Name: "CNN FHD"},
		{ID: 2, Name: "CNN UHD"},
		{ID: 3, Name: "CNN International"},
	}, 3)

	selected := Automatic(results, 0.96, 0.05)
	if len(selected) != 0 {
		t.Fatalf("Automatic() accepted ambiguous canonical duplicates: %+v", selected)
	}
}

func TestAutomaticAcceptsUniqueCanonicalMatch(t *testing.T) {
	results := Rank("CNN HD", []Candidate{
		{ID: 1, Name: "CNN FHD"},
		{ID: 2, Name: "CNN International"},
	}, 2)

	selected := Automatic(results, 0.96, 0.05)
	if len(selected) != 1 || selected[0].Candidate.ID != 1 {
		t.Fatalf("Automatic() = %+v, want the unique canonical match", selected)
	}
}

func TestAutomaticRejectsAmbiguousFuzzyMatch(t *testing.T) {
	results := []Result{
		{Candidate: Candidate{ID: 1}, Score: 0.98, RunnerUpScore: 0.95, Method: MethodFuzzyName},
		{Candidate: Candidate{ID: 2}, Score: 0.95, Method: MethodFuzzyName},
	}

	if selected := Automatic(results, 0.96, 0.05); len(selected) != 0 {
		t.Fatalf("Automatic() accepted an ambiguous result: %+v", selected)
	}
}

func TestAutomaticAcceptsClearFuzzyWinner(t *testing.T) {
	results := []Result{
		{Candidate: Candidate{ID: 1}, Score: 0.98, RunnerUpScore: 0.90, Method: MethodFuzzyName},
		{Candidate: Candidate{ID: 2}, Score: 0.90, Method: MethodFuzzyName},
	}

	selected := Automatic(results, 0.96, 0.05)
	if len(selected) != 1 || selected[0].Candidate.ID != 1 {
		t.Fatalf("Automatic() = %+v, want the clear winner", selected)
	}
}
