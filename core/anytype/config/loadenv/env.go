package loadenv

import (
	"os"

	_ "github.com/joho/godotenv/autoload"
)

func Get(key string) string {
	return os.Getenv(key)
}
