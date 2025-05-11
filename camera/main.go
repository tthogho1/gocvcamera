package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"gocv.io/x/gocv"
)

func main() {
	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		log.Fatalf("failed to initialize PortAudio: %v", err)
	}
	defer portaudio.Terminate()

	// Open the webcam with device ID 0
	webcam, err := gocv.OpenVideoCapture(0)
	if err != nil {
		panic("failed to open webcam: " + err.Error())
	}
	defer webcam.Close()

	// Create a window to display the video
	window := gocv.NewWindow("Webcam")
	defer window.Close()

	// Create a Mat for images
	img := gocv.NewMat()
	defer img.Close()

	// Create a MediaEngine object
	mediaEngine := &webrtc.MediaEngine{}

	// Register the VP8 codec
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP8,
			ClockRate:   90000,
			Channels:    0,
			SDPFmtpLine: "",
			RTCPFeedback: nil,
		},
		PayloadType: 96, // Recommended PayloadType for VP8
	}, webrtc.RTPCodecTypeVideo); err != nil {
		log.Fatalf("failed to register VP8 codec: %v", err)
	}

	// Create the API with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	// Configure ICE
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Fatalf("failed to create new PeerConnection: %v", err)
	}
	defer peerConnection.Close()

	// Create a video track
	videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "webcam")
	if err != nil {
		log.Fatalf("failed to create video track: %v", err)
	}

	// Add the video track to the PeerConnection
	rtpSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		log.Fatalf("failed to add video track: %v", err)
	}

	// Read incoming RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, err := rtpSender.Read(rtcpBuf); err != nil {
				return
			}
		}
	}()

	// WebSocket server address
	wsURL := "ws://localhost:8080"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()
	log.Println("Connected to WebSocket server")

	var clientID string

	// Receive WebSocket messages
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
				if msgType == "answer" {
					sdpMap := msg["sdp"].(map[string]interface{})
					sdpJSON, _ := json.Marshal(sdpMap)
					var answer webrtc.SessionDescription
					if err := json.Unmarshal(sdpJSON, &answer); err == nil {
						log.Println("Setting remote description (answer)")
						if err := peerConnection.SetRemoteDescription(answer); err != nil {
							log.Println("Error setting remote description:", err)
						}
					}
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
					// Create and send offer
					offer, err := peerConnection.CreateOffer(nil)
					if err != nil {
						log.Println("Error creating offer:", err)
						return
					}
					gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
					peerConnection.SetLocalDescription(offer)
					<-gatherComplete
					offerJSON, _ := json.Marshal(offer)
					conn.WriteJSON(map[string]interface{}{
						"type": "offer",
						"sdp":  json.RawMessage(offerJSON),
						"from": clientID,
					})
					log.Printf("Sent Offer SDP:\n%s\n", offer.SDP)
				}
			}
		}
	}()

	// Handle ICE candidates
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

	// Main loop to capture and send video frames
	for {
		if ok := webcam.Read(&img); !ok {
			continue
		}
		if img.Empty() {
			continue
		}

		// Convert gocv.Mat to image.Image
		image, err := img.ToImage()
		if err != nil {
			log.Printf("failed to convert gocv.Mat to image.Image: %v", err)
			continue
		}

		// **実際のアプリケーションでは、ここで image を VP8 エンコーダーでエンコードし、RTPパケットを videoTrack.WriteSample に渡す必要があります。**
		// **以下の部分はダミーデータ生成の例です。**
		width := image.Bounds().Dx()
		height := image.Bounds().Dy()
		yuv := make([]byte, width*height*3/2) // Dummy YUV420 data
		for i := 0; i < len(yuv); i++ {
			yuv[i] = byte(i % 256)
		}

		// Create a new buffer
		sample := media.Sample{
			Data:               yuv,
			Timestamp:          time.Now(),
			Duration:           time.Millisecond * 20, // 20ms per frame = 50 FPS
		}

		if err := videoTrack.WriteSample(sample); err != nil {
			// Display the image in the window
			window.IMShow(img)
		}

		// // No need to show the window if running headless
		// window.IMShow(img)
		// if window.WaitKey(1) >= 0 {
		// 	break
		// }

		time.Sleep(20 * time.Millisecond) // Simulate frame rate
	}

	// Keep the application running
	select {}
}