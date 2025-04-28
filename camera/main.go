package main

import (
	"log"

	"github.com/gordonklaus/portaudio"
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

    // Create a window
    window := gocv.NewWindow("Webcam")
    defer window.Close()

    // Create a Mat for images
    img := gocv.NewMat()
    defer img.Close()

    // Set up audio stream
    stream, err := portaudio.OpenDefaultStream(1, 1, 44100, 1024, func(in []int16,out []int16) {
        //var buf bytes.Buffer
        copy(out,in)
        //if err := binary.Write(&buf, binary.LittleEndian, in); err != nil {
        //    log.Printf("failed to write audio data: %v", err)
        //}
        // Here you can process the audio data in `buf`
        log.Printf("Audio data captured: %d samples", len(in))
    })
    if err != nil {
        log.Fatalf("failed to open audio stream: %v", err)
    }
    defer stream.Close()

    if err := stream.Start(); err != nil {
        log.Fatalf("failed to start audio stream: %v", err)
    }
    defer stream.Stop()

    // Main loop
    for {
        if ok := webcam.Read(&img); !ok {
            continue
        }
        if img.Empty() {
            continue
        }
        window.IMShow(img)
        if window.WaitKey(1) >= 0 {
            break
        }
    }
}
