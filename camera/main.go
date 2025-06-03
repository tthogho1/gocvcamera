// 基本的な構成
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"gocv.io/x/gocv"
)

// createVideoTrackFromGoCV creates a WebRTC video track from a GoCV VideoCapture,

// Configure ICE
var config = webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {
            URLs: []string{"stun:stun.l.google.com:19302"},
        },
    },
}

func main() {
    // GoCVでカメラを開く
    webcam, err := gocv.OpenVideoCapture(0)
    if err != nil {
        fmt.Println("Error opening webcam:", err)
        return
    }
    defer webcam.Close()

    // Pion WebRTCでPeerConnectionを作成
    peerConnection, _ := webrtc.NewPeerConnection(config)

    // WebSocket server address
    wsURL := "ws://localhost:8080"
    conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil {
        log.Fatalf("failed to connect to WebSocket server: %v", err)
    }
    defer conn.Close()
    log.Println("Connected to WebSocket server")

    var clientID string

    // WebRTCトラックの追加
    videoTrack, err := webrtc.NewTrackLocalStaticSample(
        webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
        "video", "pion",
    )
    if err != nil {
        panic(err)
    }
    peerConnection.AddTrack(videoTrack)

    // ICE candidate送信
    peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
        if candidate != nil {
            candidateJSON, _ := json.Marshal(candidate.ToJSON())
            conn.WriteJSON(map[string]interface{}{
                "type":      "candidate",
                "candidate": json.RawMessage(candidateJSON),
                "from":      clientID,
            })
            log.Println("Sent ICE candidate")
        }
    })

    // WebSocketメッセージ受信
    go func() {
        for {
            _, message, err := conn.ReadMessage()
            if err != nil {
                log.Println("read:", err)
                return
            }
            log.Printf("Received message from server: %s", message)
            var msg map[string]interface{}
            if err := json.Unmarshal(message, &msg); err == nil {
                msgType := msg["type"]
                if msgType == "offer" {
                    // Offerを受信したらRemoteDescriptionにセット
                    sdpMap := msg["sdp"].(map[string]interface{})
                    sdpJSON, _ := json.Marshal(sdpMap)
                    var offer webrtc.SessionDescription
                    if err := json.Unmarshal(sdpJSON, &offer); err == nil {
                        log.Println("Setting remote description (offer)")
                        if err := peerConnection.SetRemoteDescription(offer); err != nil {
                            log.Println("Error setting remote description:", err)
                            return
                        }
                    }
                    // Answerを生成して送信
                    answer, err := peerConnection.CreateAnswer(nil)
                    if err != nil {
                        log.Println("Error creating answer:", err)
                        return
                    }
                    gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
                    peerConnection.SetLocalDescription(answer)
                    <-gatherComplete
                    answerJSON, _ := json.Marshal(answer)
                    conn.WriteJSON(map[string]interface{}{
                        "type": "answer",
                        "sdp":  json.RawMessage(answerJSON),
                        "from": clientID,
                    })
                    log.Printf("Sent Answer SDP:\n%s\n", answer.SDP)
                } else if msgType == "candidate" {
                    candidateMap := msg["candidate"].(map[string]interface{})
                    candidateJSON, _ := json.Marshal(candidateMap)
                    var candidate webrtc.ICECandidateInit
                    if err := json.Unmarshal(candidateJSON, &candidate); err == nil {
                        log.Println("Adding ICE candidate")
                        if err := peerConnection.AddICECandidate(candidate); err != nil {
                            log.Println("Error adding ICE candidate:", err)
                        }
                    }
                } else if msgType == "connected" {
                    clientID = msg["id"].(string)
                    log.Println("My Client ID:", clientID)
                }
            }
        }
    }()

    // カメラ画像の送信・表示
    window := gocv.NewWindow("WebRTC Video")
    defer window.Close()
    img := gocv.NewMat()
    defer img.Close()

    for {
        if ok := webcam.Read(&img); !ok || img.Empty() {
            fmt.Println("Error reading from webcam")
            continue
        }
        window.IMShow(img)
        if window.WaitKey(1) >= 0 {
            break
        }
        sample := media.Sample{
            Data:    img.ToBytes(),
            Duration: 33 * 1e6, // 約30FPS
        }
        if err := videoTrack.WriteSample(sample); err != nil {
            panic(err)
        }
    }
}

