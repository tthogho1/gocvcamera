package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"

	"github.com/pion/webrtc/v4"
	"golang.org/x/net/websocket"
)

const (
	ayameWebSocketURL = "wss://ayame-labo.shiguredo.jp/signaling"
	roomID            = "p2p-room" // 同じルームIDを使用
	signalingKey      = ""
	isInitiator       = true // このピアが Offer を作成するイニシエータ
)

func main() {
	// Pion の設定
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// PeerConnection の作成
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Fatalf("PeerConnection の作成に失敗しました: %v", err)
	}
	defer peerConnection.Close()

	// データチャネルの作成 (イニシエータのみ)
	var dataChannel *webrtc.DataChannel
	if isInitiator {
		dataChannel, err = peerConnection.CreateDataChannel("data", nil)
		if err != nil {
			log.Fatalf("データチャネルの作成に失敗しました: %v", err)
		}
		dataChannel.OnOpen(func() {
			log.Println("データチャネル (イニシエータ) が開きました")
			sendMessage(dataChannel, "ピア B にこんにちは！ (from A)")
		})
	} else {
		peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
			log.Println("データチャネル (非イニシエータ) を受信しました")
			dataChannel = dc
			dataChannel.OnOpen(func() {
				sendMessage(dataChannel, "ピア A にこんにちは！ (from B)")
			})
			dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
				log.Printf("データチャネル (非イニシエータ) でメッセージを受信しました: %s", string(msg.Data))
			})
		})
	}

	// データチャネルのメッセージ受信
	if isInitiator && dataChannel != nil {
		dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("データチャネル (イニシエータ) でメッセージを受信しました: %s", string(msg.Data))
		})
	}

	// WebSocket で Ayame Labo に接続
	origin := "http://localhost/"
	ws, err := websocket.Dial(ayameWebSocketURL, "", origin)
	if err != nil {
		log.Fatalf("WebSocket 接続に失敗しました: %v", err)
	}
	defer ws.Close()
	log.Println("WebSocket 接続に成功しました (ピア A)")

	// Ayame Labo への登録
	registerMessage := map[string]interface{}{
		"type":         "register",
		"room_id":      roomID,
		"signaling_key": signalingKey,
	}
	registerJSON, _ := json.Marshal(registerMessage)
	_, err = ws.Write(registerJSON)
	log.Printf("登録要求を送信しました (ピア A): %s", string(registerJSON))

	// Offer/Answer 交換
	if isInitiator {
		// Offer を作成
		offer, err := peerConnection.CreateOffer(nil)
		if err != nil {
			log.Fatalf("Offer の作成に失敗しました: %v", err)
		}
		err = peerConnection.SetLocalDescription(offer)
		if err != nil {
			log.Fatalf("Local Description の設定に失敗しました: %v", err)
		}

		gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
		<-gatherComplete

		offerMessage := map[string]interface{}{
			"type": "offer",
			"sdp":  offer.SDP,
		}
		offerJSON, _ := json.Marshal(offerMessage)
		_, err = ws.Write(offerJSON)
		log.Printf("Offer を送信しました (ピア A): %s", string(offerJSON))
	}

	// Signaling メッセージの受信
	go func() {
		for {
			msg := make([]byte, 2048)
			n, err := ws.Read(msg)
			if err != nil {
				log.Printf("WebSocket 受信エラー (ピア A): %v", err)
				return
			}
			if n > 0 {
				var signalingMessage map[string]interface{}
				json.Unmarshal(msg[:n], &signalingMessage)
				log.Printf("Signaling メッセージを受信しました (ピア A): %v", signalingMessage)

				switch signalingMessage["type"] {
				case "answer":
					answer := webrtc.SessionDescription{
						Type: webrtc.SDPTypeAnswer,
						SDP: signalingMessage["sdp"].(string),
					}
					err = peerConnection.SetRemoteDescription(answer)
					if err != nil {
						log.Printf("Remote Description の設定に失敗しました (ピア A): %v", err)
					}
				case "candidate":
					if candidateJSON, ok := signalingMessage["candidate"].(map[string]interface{}); ok {
						candidate := webrtc.ICECandidateInit{
							Candidate:     candidateJSON["candidate"].(string),
							SDPMLineIndex: func(v uint16) *uint16 { return &v }(uint16(candidateJSON["sdpMLineIndex"].(float64))),
							SDPMid:        func(s string) *string { return &s }(candidateJSON["sdpMid"].(string)),
						}
						err = peerConnection.AddICECandidate(candidate)
						if err != nil {
							log.Printf("ICE Candidate の追加に失敗しました (ピア A): %v", err)
						}
					}
				case "ping":
					pongMessage := map[string]interface{}{"type": "pong"}
					pongJSON, _ := json.Marshal(pongMessage)
					ws.Write(pongJSON)
				}
			}
		}
	}()

	// ICE Candidate の収集
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		candidateJSON := map[string]interface{}{
			"type": "candidate",
			"candidate": map[string]interface{}{
				"candidate":     candidate.String(),
				"sdpMLineIndex": candidate.SDPMLineIndex,
				"sdpMid":        candidate.SDPMid,
			},
		}
		payload, _ := json.Marshal(candidateJSON)
		ws.Write(payload)
	})

	// Ctrl+C で終了
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	log.Println("終了します (ピア A)...")
}

// 任意のタイミングでテキストメッセージを送信する関数
func sendMessage(dc *webrtc.DataChannel, text string) {
	if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
		err := dc.SendText(text)
		if err != nil {
			log.Printf("データ送信エラー: %v", err)
		} else {
			log.Printf("データを送信しました: %s", text)
		}
	} else {
		log.Println("データチャネルがオープンではありません")
	}
}