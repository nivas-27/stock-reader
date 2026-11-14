package env

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func CheckEnv() string {
	err := godotenv.Load()
	if err != nil {
		// fmt.Println("Error loading .env file but will try from the environment variable")
	}
	fmt.Println("Environment variables initialized.")
	return "success"
}

func GetEnv(key string) string {
	val := os.Getenv(key)

	if val == "" {
		fmt.Printf("[ENV WARNING]: %s is not set\n", key)
	}
	return val
}
