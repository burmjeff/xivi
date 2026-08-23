package channelmatch

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const CurrentVersion = 2

const (
	MethodExactName    = "exact_name"
	MethodFuzzyName    = "fuzzy_name"
	MethodExactTvgID   = "exact_tvg_id"
	MethodTvgIDName    = "exact_tvg_id_name"
	MethodManual       = "manual"
	MethodLegacy       = "legacy"
	defaultMinRunes    = 3
	baseNameExactScore = 0.99
)

type Candidate struct {
	ID   int64
	Name string
}

type Result struct {
	Candidate     Candidate
	Score         float64
	RunnerUpScore float64
	Method        string
}

type ParsedName struct {
	Canonical string
	Base      string
	Tokens    []string
	Numbers   []string
	Locales   []string
	Regions   []string
	Feeds     []string
}

// Automatic returns only results that are safe enough to commit without a
// person reviewing them. Even a canonical match must be unique; fuzzy matches
// must clear both an absolute threshold and a runner-up margin.
func Automatic(results []Result, threshold, minimumMargin float64) []Result {
	if len(results) == 0 {
		return nil
	}

	if results[0].Method == MethodExactName {
		exactCount := 0
		for _, result := range results {
			if result.Method != MethodExactName || result.Score != 1 {
				break
			}
			exactCount++
		}
		if exactCount == 1 {
			return results[:1]
		}
		return nil
	}

	best := results[0]
	if best.Score < threshold || best.Score-best.RunnerUpScore < minimumMargin {
		return nil
	}
	return []Result{best}
}

var ignoredQualityTokens = map[string]struct{}{
	"2k": {}, "4k": {}, "8k": {}, "1080p": {}, "720p": {},
	"backup": {}, "dead": {}, "fd": {}, "fhd": {}, "h264": {}, "h265": {},
	"hd": {}, "hevc": {}, "hq": {}, "lq": {}, "m3u8": {}, "qhd": {}, "raw": {},
	"rec": {}, "sd": {}, "slow": {}, "uhd": {}, "unknown": {}, "unk": {},
	"50fps": {}, "60fps": {},
}

var genericSuffixTokens = map[string]struct{}{
	"channel": {}, "network": {}, "tv": {}, "television": {},
}

var feedTokens = map[string]struct{}{
	"east": {}, "west": {}, "north": {}, "south": {},
}

var localeTokens = map[string]struct{}{
	"ar": {}, "at": {}, "au": {}, "be": {}, "br": {}, "ca": {}, "ch": {},
	"cn": {}, "de": {}, "dk": {}, "en": {}, "es": {}, "fi": {}, "fr": {},
	"gb": {}, "gr": {}, "ie": {}, "in": {}, "int": {}, "it": {}, "jp": {},
	"kr": {}, "mx": {}, "nl": {}, "no": {}, "nz": {}, "pl": {}, "pt": {},
	"ru": {}, "se": {}, "tr": {}, "uk": {}, "us": {}, "za": {},
}

var regionTokens = map[string]struct{}{
	"al": {}, "ak": {}, "az": {}, "ar": {}, "ca": {}, "co": {}, "ct": {},
	"de": {}, "fl": {}, "ga": {}, "hi": {}, "id": {}, "il": {}, "in": {},
	"ia": {}, "ks": {}, "ky": {}, "la": {}, "me": {}, "md": {}, "ma": {},
	"mi": {}, "mn": {}, "ms": {}, "mo": {}, "mt": {}, "ne": {}, "nv": {},
	"nh": {}, "nj": {}, "nm": {}, "ny": {}, "nc": {}, "nd": {}, "oh": {},
	"ok": {}, "or": {}, "pa": {}, "ri": {}, "sc": {}, "sd": {}, "tn": {},
	"tx": {}, "ut": {}, "vt": {}, "va": {}, "wa": {}, "wv": {}, "wi": {},
	"wy": {}, "dc": {},
}

var writtenNumbers = map[string]string{
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4",
	"five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9",
	"ten": "10", "eleven": "11", "twelve": "12", "thirteen": "13",
	"fourteen": "14", "fifteen": "15", "sixteen": "16", "seventeen": "17",
	"eighteen": "18", "nineteen": "19", "twenty": "20",
	"ein": "1", "eins": "1", "zwei": "2", "deux": "2", "dos": "2",
}

var romanNumbers = map[string]string{
	"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5",
	"vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10",
	"xi": "11", "xii": "12", "xiii": "13", "xiv": "14", "xv": "15",
	"xvi": "16", "xvii": "17", "xviii": "18", "xix": "19", "xx": "20",
}

var caseFolder = cases.Fold()

func NormalizeTvgID(value string) string {
	return strings.TrimSpace(foldAndNormalize(value))
}

func ParseName(value string) ParsedName {
	normalized := foldAndNormalize(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "&", " and ")
	normalized = strings.ReplaceAll(normalized, "+", " plus ")

	normalized = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return ' '
	}, normalized)

	rawTokens := strings.Fields(normalized)
	tokens := make([]string, 0, len(rawTokens))
	for index, token := range rawTokens {
		_, ignored := ignoredQualityTokens[token]
		if ignored && !(token == "sd" && isLocalRegionSuffix(rawTokens, index)) {
			continue
		}
		if number, ok := writtenNumbers[token]; ok {
			token = number
		}
		tokens = append(tokens, normalizeNumericToken(token))
	}

	if len(tokens) > 1 {
		if number, ok := romanNumbers[tokens[len(tokens)-1]]; ok {
			tokens[len(tokens)-1] = number
		}
	}
	if len(tokens) > 1 && tokens[0] == "the" {
		tokens = tokens[1:]
	}

	parsed := ParsedName{
		Canonical: strings.Join(tokens, " "),
		Tokens:    append([]string(nil), tokens...),
	}

	for index, token := range tokens {
		if isNumeric(token) {
			parsed.Numbers = append(parsed.Numbers, token)
		}
		if _, ok := feedTokens[token]; ok {
			parsed.Feeds = append(parsed.Feeds, token)
		}
		if (index == 0 || index == len(tokens)-1) && isLocale(token) {
			parsed.Locales = append(parsed.Locales, token)
		}
		if index == len(tokens)-1 {
			if _, ok := regionTokens[token]; ok {
				parsed.Regions = append(parsed.Regions, token)
			}
		}
	}

	baseTokens := append([]string(nil), tokens...)
	if len(baseTokens) > 0 && isLocale(baseTokens[0]) {
		baseTokens = baseTokens[1:]
	}
	if len(baseTokens) > 1 {
		if _, generic := genericSuffixTokens[baseTokens[len(baseTokens)-1]]; generic {
			baseTokens = baseTokens[:len(baseTokens)-1]
		}
	}
	parsed.Base = strings.Join(baseTokens, " ")

	parsed.Numbers = uniqueSorted(parsed.Numbers)
	parsed.Locales = uniqueSorted(parsed.Locales)
	parsed.Regions = uniqueSorted(parsed.Regions)
	parsed.Feeds = uniqueSorted(parsed.Feeds)
	return parsed
}

func foldAndNormalize(value string) string {
	return norm.NFKC.String(caseFolder.String(norm.NFKC.String(value)))
}

func isLocalRegionSuffix(tokens []string, index int) bool {
	if index != len(tokens)-1 {
		return false
	}
	hasNews := false
	hasNumber := false
	for _, token := range tokens[:index] {
		hasNews = hasNews || token == "news"
		hasNumber = hasNumber || isNumeric(token)
	}
	return hasNews && hasNumber
}

func Rank(query string, candidates []Candidate, limit int) []Result {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}

	parsedQuery := ParseName(query)
	if parsedQuery.Canonical == "" {
		return nil
	}

	results := make([]Result, 0, len(candidates))
	for _, candidate := range candidates {
		parsedCandidate := ParseName(candidate.Name)
		score, method, ok := score(parsedQuery, parsedCandidate)
		if !ok {
			continue
		}
		results = append(results, Result{
			Candidate: candidate,
			Score:     score,
			Method:    method,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Candidate.ID < results[j].Candidate.ID
		}
		return results[i].Score > results[j].Score
	})

	for index := range results {
		if index+1 < len(results) {
			results[index].RunnerUpScore = results[index+1].Score
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func score(query, candidate ParsedName) (float64, string, bool) {
	if query.Canonical == candidate.Canonical {
		return 1, MethodExactName, true
	}
	if !compatible(query, candidate) {
		return 0, "", false
	}
	if query.Base != "" && query.Base == candidate.Base {
		return baseNameExactScore * missingAttributePenalty(query, candidate), MethodFuzzyName, true
	}
	if utf8.RuneCountInString(query.Base) < defaultMinRunes || utf8.RuneCountInString(candidate.Base) < defaultMinRunes {
		return 0, "", false
	}

	characterScore := ngramDice(compact(query.Base), compact(candidate.Base), 3)
	tokenScore := tokenJaccard(strings.Fields(query.Base), strings.Fields(candidate.Base))
	combined := (0.75*characterScore + 0.25*tokenScore) * missingAttributePenalty(query, candidate)
	if combined <= 0 {
		return 0, "", false
	}
	return combined, MethodFuzzyName, true
}

func compatible(left, right ParsedName) bool {
	return !conflicts(left.Numbers, right.Numbers) &&
		!conflicts(left.Locales, right.Locales) &&
		!conflicts(left.Regions, right.Regions) &&
		!conflicts(left.Feeds, right.Feeds)
}

func conflicts(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	return !sameStrings(left, right)
}

func missingAttributePenalty(left, right ParsedName) float64 {
	penalty := 1.0
	if oneMissing(left.Numbers, right.Numbers) {
		penalty *= 0.90
	}
	if oneMissing(left.Locales, right.Locales) {
		penalty *= 0.97
	}
	if oneMissing(left.Regions, right.Regions) {
		penalty *= 0.85
	}
	if oneMissing(left.Feeds, right.Feeds) {
		penalty *= 0.85
	}
	return penalty
}

func oneMissing(left, right []string) bool {
	return (len(left) == 0) != (len(right) == 0)
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func normalizeNumericToken(token string) string {
	if !isNumeric(token) {
		return token
	}
	number, err := strconv.ParseUint(token, 10, 64)
	if err != nil {
		return token
	}
	return strconv.FormatUint(number, 10)
}

func isNumeric(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if !unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

func isLocale(token string) bool {
	_, ok := localeTokens[token]
	return ok
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func compact(value string) string {
	return strings.ReplaceAll(value, " ", "")
}

func ngramDice(left, right string, size int) float64 {
	leftSet := ngrams(left, size)
	rightSet := ngrams(right, size)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for item := range leftSet {
		if _, ok := rightSet[item]; ok {
			intersection++
		}
	}
	return float64(2*intersection) / float64(len(leftSet)+len(rightSet))
}

func ngrams(value string, size int) map[string]struct{} {
	runes := []rune(value)
	result := make(map[string]struct{})
	if len(runes) < size {
		return result
	}
	for index := 0; index <= len(runes)-size; index++ {
		result[string(runes[index:index+size])] = struct{}{}
	}
	return result
}

func tokenJaccard(left, right []string) float64 {
	leftSet := make(map[string]struct{}, len(left))
	rightSet := make(map[string]struct{}, len(right))
	for _, token := range left {
		leftSet[token] = struct{}{}
	}
	for _, token := range right {
		rightSet[token] = struct{}{}
	}
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for token := range leftSet {
		if _, ok := rightSet[token]; ok {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	return float64(intersection) / float64(union)
}
