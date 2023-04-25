package loadenv

import (
	_ "github.com/joho/godotenv/autoload"
	"os"
)

func Get(key string) string {
	return os.Getenv(key)
}
