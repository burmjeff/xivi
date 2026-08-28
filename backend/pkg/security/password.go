package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 1
	argonSaltLength  = 16
	argonKeyLength   = 32
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

var (
	ErrPasswordVerifierBusy = errors.New("password verification capacity is busy")
	passwordVerifierSlots   = make(chan struct{}, 4)
)

var commonPasswords = map[string]struct{}{
	"password": {}, "password123": {}, "123456789": {}, "qwerty123": {},
	"letmein": {}, "admin": {}, "administrator": {}, "welcome": {}, "xivi": {},
	"passwordpassword": {}, "password123456": {}, "123456789012345": {},
	"qwertyuiopasdfg": {}, "letmeinletmein": {}, "administrator123": {},
	"welcome123456789": {}, "changemechangeme": {},
}

func NormalizeUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !usernamePattern.MatchString(value) {
		return "", errors.New("username must be 3-64 lowercase letters, numbers, dots, underscores, or hyphens")
	}
	return value, nil
}

// NormalizeDisplayName keeps display names presentation-only while bounding
// their storage and excluding control characters that do not belong in UI or
// audit output. An empty value is valid and means "show the username".
func NormalizeDisplayName(value string) (string, error) {
	value = strings.TrimSpace(norm.NFC.String(value))
	if utf8.RuneCountInString(value) > 80 {
		return "", errors.New("display name must contain at most 80 characters")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New("display name cannot contain control characters")
		}
	}
	return value, nil
}

func ValidatePassword(password, username string) error {
	length := utf8.RuneCountInString(password)
	if length < 8 || length > 128 {
		return errors.New("password must contain between 8 and 128 characters")
	}
	lower := strings.ToLower(strings.TrimSpace(password))
	var compact strings.Builder
	for _, value := range lower {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			compact.WriteRune(value)
		}
	}
	compacted := compact.String()
	commonPattern := false
	for _, fragment := range []string{"password", "qwerty", "letmein", "changeme", "administrator", "welcome", "1234567890", "abcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(compacted, fragment) {
			commonPattern = true
			break
		}
	}
	allSame := true
	var first rune
	for index, value := range []rune(lower) {
		if index == 0 {
			first = value
		} else if value != first {
			allSame = false
			break
		}
	}
	if _, found := commonPasswords[lower]; found || commonPattern || allSame || (len(username) >= 3 && strings.Contains(lower, strings.ToLower(username))) {
		return errors.New("password is too common or contains the username")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonIterations,
		argonParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	select {
	case passwordVerifierSlots <- struct{}{}:
		defer func() { <-passwordVerifierSlots }()
	default:
		// Argon2 deliberately consumes memory. A global ceiling prevents a
		// distributed login flood from multiplying that cost without bound.
		return false, ErrPasswordVerifierBusy
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("password hash format is invalid")
	}
	var memory uint64
	var iterations uint64
	var parallelism uint64
	for _, value := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(value, "=", 2)
		if len(pair) != 2 {
			return false, errors.New("password hash parameters are invalid")
		}
		parsed, err := strconv.ParseUint(pair[1], 10, 32)
		if err != nil {
			return false, err
		}
		switch pair[0] {
		case "m":
			memory = parsed
		case "t":
			iterations = parsed
		case "p":
			parallelism = parsed
		}
	}
	if memory < 19*1024 || memory > 1024*1024 || iterations < 1 || iterations > 20 || parallelism < 1 || parallelism > 16 {
		return false, errors.New("password hash parameters are outside safe bounds")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 {
		return false, errors.New("password hash salt is invalid")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false, errors.New("password hash is invalid")
	}
	actual := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(want, actual) == 1, nil
}

func DummyPasswordHash() string {
	// A valid fixed hash makes unknown-user login cost comparable to a real one.
	return "$argon2id$v=19$m=65536,t=3,p=1$c2FsdC1zYWx0LXNhbHQxMg$M6C8LLWHip5iXxR2o2prRxtQfgE/pySnJT7HXyXwBLY"
}
