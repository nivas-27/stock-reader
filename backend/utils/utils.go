package utils

import (
	env "backend/configs/env"
	"crypto/sha256"
	"encoding/hex"
)

func GenerateChecksum() string {
	apiKey := env.GetEnv("KITE_API_KEY")
	apiSecret := env.GetEnv("KITE_API_SECRET")
	requestToken := env.GetEnv("KITE_REQUEST_TOKEN")

	println("requestToken: " + requestToken)
	data := apiKey + requestToken + apiSecret
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
