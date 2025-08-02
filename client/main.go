package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"net"
	"os"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/nfnt/resize"
)

const (
	SERVER_ADDR = "192.168.1.4:8888" // Địa chỉ server
	MAX_SIZE    = 65507              // Giới hạn kích thước gói UDP
)

func captureScreenshot() ([]byte, error) {
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, fmt.Errorf("no active display")
	}
	img, err := screenshot.CaptureDisplay(0)
	if err != nil {
		return nil, err
	}

	// Resize ảnh để giảm kích thước (giảm 50% độ phân giải)
	resizedImg := resize.Resize(uint(img.Bounds().Dx()/2), 0, img, resize.Lanczos3)

	// Mã hóa sang JPEG với chất lượng 70
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 70})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getHostnameAndIP() (string, string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", "", err
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return hostname, ipnet.IP.String(), nil
			}
		}
	}
	return hostname, "unknown", nil
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", SERVER_ADDR)
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	defer conn.Close()

	hostname, ip, err := getHostnameAndIP()
	if err != nil {
		fmt.Println("Error getting hostname/IP:", err)
		return
	}
	CLIENT_ID := ip // Sử dụng IP làm CLIENT_ID

	// Nhận ACK từ server
	go func() {
		listener, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
		if err != nil {
			fmt.Println("Error listening:", err)
			return
		}
		defer listener.Close()
		buf := make([]byte, MAX_SIZE)
		for {
			n, _, err := listener.ReadFromUDP(buf)
			if err != nil {
				fmt.Println("Error reading:", err)
				continue
			}
			data := string(buf[:n])
			if data == "ACK:"+CLIENT_ID {
				fmt.Println("Received ACK from server")
			}
		}
	}()

	// Gửi ảnh màn hình
	for {
		imgData, err := captureScreenshot()
		if err != nil {
			fmt.Println("Error capturing screenshot:", err)
			time.Sleep(60 * time.Second)
			continue
		}
		header := fmt.Sprintf("SCREENSHOT:%s:%s:%s:%d:", CLIENT_ID, hostname, ip, len(imgData))
		packet := append([]byte(header), imgData...)
		if len(packet) > MAX_SIZE {
			fmt.Println("Packet too large, skipping")
			time.Sleep(60 * time.Second)
			continue
		}
		_, err = conn.Write(packet)
		if err != nil {
			fmt.Println("Error sending screenshot:", err)
		} else {
			fmt.Println("Sent screenshot to server")
		}
		time.Sleep(60 * time.Second)
	}
}
