package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"

	"golang.org/x/term"
)

func IsAuthCLI(args []string) bool { return len(args) > 0 && args[0] == "auth" }
func IsGenerateKeyCLI(args []string) bool {
	return len(args) > 1 && args[0] == "auth" && args[1] == "generate-key"
}

func RunAuthCLI(args []string) error {
	if !IsAuthCLI(args) || len(args) < 2 {
		return fmt.Errorf("usage: xivi auth <generate-key|reset-password|disable-mfa>")
	}
	if args[1] == "generate-key" {
		set := flag.NewFlagSet("auth generate-key", flag.ContinueOnError)
		output := set.String("output", security.AuthKeyPath(), "authentication key output path")
		if err := set.Parse(args[2:]); err != nil {
			return err
		}
		if err := security.GenerateKeyFile(*output); err != nil {
			return err
		}
		fmt.Printf("Authentication key created at %s\n", *output)
		return nil
	}

	db, err := database.OpenDBConnection()
	if err != nil {
		return err
	}
	database.Db = db
	defer database.CloseDBConnection()

	switch args[1] {
	case "reset-password":
		set := flag.NewFlagSet("auth reset-password", flag.ContinueOnError)
		usernameFlag := set.String("username", "", "account username")
		if err := set.Parse(args[2:]); err != nil {
			return err
		}
		username := strings.TrimSpace(*usernameFlag)
		if username == "" {
			username, err = promptLine("Username: ")
		}
		if err != nil {
			return err
		}
		user, err := database.Db.GetUserByUsername(context.Background(), username)
		if err != nil {
			return fmt.Errorf("account not found")
		}
		password, err := promptPasswordTwice()
		if err != nil {
			return err
		}
		if err := security.ValidatePassword(password, user.Username); err != nil {
			return err
		}
		hash, err := security.HashPassword(password)
		if err != nil {
			return err
		}
		if err := database.Db.SetUserPassword(context.Background(), user.ID, hash, true); err != nil {
			return err
		}
		_ = database.Db.RevokeUserSessions(context.Background(), user.ID)
		_ = database.Db.RevokeUserMediaKeys(context.Background(), user.ID)
		fmt.Printf("Password reset for %s. The account must change it after signing in.\n", user.Username)
		return nil
	case "disable-mfa":
		set := flag.NewFlagSet("auth disable-mfa", flag.ContinueOnError)
		usernameFlag := set.String("username", "", "account username")
		if err := set.Parse(args[2:]); err != nil {
			return err
		}
		username := strings.TrimSpace(*usernameFlag)
		if username == "" {
			username, err = promptLine("Username: ")
		}
		if err != nil {
			return err
		}
		user, err := database.Db.GetUserByUsername(context.Background(), username)
		if err != nil {
			return fmt.Errorf("account not found")
		}
		if err := database.Db.DisableMFA(context.Background(), user.ID); err != nil {
			return err
		}
		_ = database.Db.RevokeUserSessions(context.Background(), user.ID)
		fmt.Printf("MFA disabled for %s; all browser sessions were revoked.\n", user.Username)
		return nil
	default:
		return fmt.Errorf("unknown auth command %q", args[1])
	}
}

func promptLine(label string) (string, error) {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	return strings.TrimSpace(value), err
}

func promptPasswordTwice() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("password input requires an interactive terminal")
	}
	fmt.Print("Password: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("Confirm password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", fmt.Errorf("passwords did not match")
	}
	return string(first), nil
}
