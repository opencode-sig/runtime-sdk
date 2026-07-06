package mysql

import "fmt"

func validateIdentifier(value string) error {
	if value == "" {
		return fmt.Errorf("identifier is required")
	}
	for _, r := range value {
		if r == '_' ||
			(r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') {
			continue
		}
		return fmt.Errorf("identifier %q contains unsupported character %q", value, r)
	}
	return nil
}

func quoteIdentifier(value string) (string, error) {
	if err := validateIdentifier(value); err != nil {
		return "", err
	}
	return "`" + value + "`", nil
}
