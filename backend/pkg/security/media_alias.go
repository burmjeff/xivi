package security

import (
	"crypto/rand"
	"strings"
)

const (
	MediaOutputCodeSymbols = 16
	mediaOutputAlphabet    = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// GenerateMediaOutputCode returns 80 bits of CSPRNG entropy encoded as four
// case-insensitive Crockford Base32 groups. The display value is exactly 19
// characters; NormalizeMediaOutputCode removes presentation separators before
// hashing and lookup.
func GenerateMediaOutputCode() (string, error) {
	random := make([]byte, MediaOutputCodeSymbols)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	compact := make([]byte, MediaOutputCodeSymbols)
	for index, value := range random {
		compact[index] = mediaOutputAlphabet[int(value)&31]
	}
	return FormatMediaOutputCode(string(compact)), nil
}

func FormatMediaOutputCode(compact string) string {
	compact = strings.ToUpper(strings.TrimSpace(compact))
	if len(compact) != MediaOutputCodeSymbols {
		return compact
	}
	return compact[0:4] + "-" + compact[4:8] + "-" + compact[8:12] + "-" + compact[12:16]
}

func NormalizeMediaOutputCode(value string) (string, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))
	compact := make([]byte, 0, MediaOutputCodeSymbols)
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '-' {
			continue
		}
		// Crockford Base32 permits these human-friendly aliases when decoding.
		switch character {
		case 'O':
			character = '0'
		case 'I', 'L':
			character = '1'
		}
		if !strings.ContainsRune(mediaOutputAlphabet, rune(character)) {
			return "", false
		}
		compact = append(compact, character)
	}
	if len(compact) != MediaOutputCodeSymbols {
		return "", false
	}
	return string(compact), true
}
