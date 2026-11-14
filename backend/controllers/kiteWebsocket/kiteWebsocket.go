package websocket

import (
	"backend/configs/env"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

var TOKENS = []int{738561, 2953217} // example: RELIANCE, TCS

func ConnectWebSocket() {
	// Initialize API credentials after env.CheckEnv() is called
	API_KEY := env.GetEnv("KITE_API_KEY")
	ACCESS_TOKEN := env.GetEnv("KITE_ACCESS_TOKEN")

	// 1. Connect WebSocket
	wsURL := fmt.Sprintf(
		"wss://ws.kite.trade?api_key=%s&access_token=%s",
		API_KEY,
		ACCESS_TOKEN,
	)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Println("WebSocket connection failed:", err)
		return
	}
	defer conn.Close()

	log.Println("Connected to Kite WebSocket")

	// 2. Subscribe to tokens
	subscribeMsg := map[string]interface{}{
		"a": "subscribe",
		"v": TOKENS,
	}
	if err := conn.WriteJSON(subscribeMsg); err != nil {
		log.Println("Subscribe error:", err)
		return
	}

	// 3. Set mode (ltp = lightweight)
	modeMsg := map[string]interface{}{
		"a": "mode",
		"v": []interface{}{"ltp", TOKENS},
	}
	if err := conn.WriteJSON(modeMsg); err != nil {
		log.Println("Mode error:", err)
		return
	}

	log.Println("Subscribed to tokens:", TOKENS)

	// 4. Read messages
	for {
		msgType, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		// Kite sends binary packets
		if msgType == websocket.BinaryMessage {
			parseBinary(message)
		} else {
			log.Println("Text:", string(message))
		}
	}
}

// 5. Minimal binary parser (LTP mode)
func parseBinary(data []byte) {
	if len(data) < 2 {
		return
	}

	// number of packets
	packetCount := int(binary.BigEndian.Uint16(data[0:2]))
	offset := 2

	for i := 0; i < packetCount; i++ {
		if offset+2 > len(data) {
			break
		}

		packetLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if offset+packetLength > len(data) {
			break
		}

		packet := data[offset : offset+packetLength]
		offset += packetLength

		if len(packet) < 8 {
			continue
		}

		// first 4 bytes = instrument token
		token := binary.BigEndian.Uint32(packet[0:4])

		// next 4 bytes = last traded price * 100
		price := binary.BigEndian.Uint32(packet[4:8])
		ltp := float64(price) / 100.0

		fmt.Printf("Token: %d | LTP: %.2f\n", token, ltp)
	}
}
