package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image/jpeg"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
)

//go:embed static/index.html
var indexHTML []byte

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var server *http.Server
var serverRunning bool
var serverMux sync.Mutex

func captureScreen() ([]byte, error) {
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, fmt.Errorf("failed to capture screen: %v", err)
	}

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, nil); err != nil {
		return nil, fmt.Errorf("failed to encode image to JPEG: %v", err)
	}

	return buffer.Bytes(), nil
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to upgrade to websocket:", err)
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		frame, err := captureScreen()
		if err != nil {
			log.Println("Error capturing screen:", err)
			continue
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			log.Println("Failed to send message:", err)
			break
		}
	}
}

func getLocalIPAddress() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no suitable IP address found")
}

func startServer() error {
	serverMux.Lock()
	defer serverMux.Unlock()

	if serverRunning {
		return fmt.Errorf("server is already running")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexHTML)
	})

	mux.HandleFunc("/ws", websocketHandler)

	server = &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on :8080: %v\n", err)
		}
	}()

	serverRunning = true
	return nil
}

func stopServer() error {
	serverMux.Lock()
	defer serverMux.Unlock()

	if !serverRunning {
		return fmt.Errorf("server is not running")
	}

	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return err
		}
		serverRunning = false
	}

	return nil
}

func main() {
	a := app.New()
	w := a.NewWindow("LANCast Server")

	ipAddress, err := getLocalIPAddress()
	if err != nil {
		log.Fatal("Failed to get local IP address:", err)
	}

	ipLabel := widget.NewLabel(fmt.Sprintf("Server IP: %s:8080", ipAddress))

	startButton := widget.NewButton("Start Server", func() {
		if !serverRunning {
			if err := startServer(); err != nil {
				log.Println("Failed to start server:", err)
			} else {
				ipLabel.SetText(fmt.Sprintf("Server running at: %s:8080", ipAddress))
			}
		}
	})

	stopButton := widget.NewButton("Stop Server", func() {
		if serverRunning {
			if err := stopServer(); err != nil {
				log.Println("Failed to stop server:", err)
			} else {
				ipLabel.SetText(fmt.Sprintf("Server stopped. Last IP: %s:8080", ipAddress))
			}
		}
	})

	content := container.NewVBox(
		ipLabel,
		startButton,
		stopButton,
	)

	w.SetContent(content)
	w.Resize(fyne.NewSize(400, 200))
	w.ShowAndRun()
}
