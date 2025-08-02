package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	HOST     = "0.0.0.0:8888"
	SAVE_DIR = "MayHS"
	CSV_FILE = "metadata.csv"
	MAX_SIZE = 65507
)

type ClientInfo struct {
	Hostname string
	IP       string
}

var (
	clients = make(map[string]ClientInfo) // Lưu client_id -> hostname, ip
	mu      sync.Mutex
)

func saveScreenshot(clientID, hostname, ip string, imgData []byte) {
	// Thay thế các ký tự không hợp lệ trong hostname để làm tên thư mục
	safeHostname := strings.ReplaceAll(hostname, string(os.PathSeparator), "_")
	safeHostname = strings.ReplaceAll(safeHostname, ":", "_")

	// Tạo thư mục riêng cho client dựa trên hostname
	clientDir := filepath.Join(SAVE_DIR, safeHostname)
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		fmt.Println("Error creating directory for", safeHostname, ":", err)
		return
	}

	// Lưu ảnh vào thư mục riêng
	filename := filepath.Join(clientDir, fmt.Sprintf("%s_%s_%d.jpg", safeHostname, ip, time.Now().Unix()))
	if err := os.WriteFile(filename, imgData, 0644); err != nil {
		fmt.Println("Error saving screenshot:", err)
		return
	}
	fmt.Println("Saved screenshot from", clientID, "to", filename)

	// Lưu metadata vào CSV
	f, err := os.OpenFile(CSV_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening CSV:", err)
		return
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	writer.Write([]string{clientID, hostname, ip, time.Now().Format(time.RFC3339), filename})
	writer.Flush()
}

func main() {
	if err := os.MkdirAll(SAVE_DIR, 0755); err != nil {
		fmt.Println("Error creating directory:", err)
		return
	}

	addr, err := net.ResolveUDPAddr("udp", HOST)
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Server listening on", HOST)

	// Xử lý gói tin UDP
	go func() {
		buf := make([]byte, MAX_SIZE)
		for {
			n, clientAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				fmt.Println("Error reading:", err)
				continue
			}
			if n == 0 {
				fmt.Println("Empty packet received")
				continue
			}
			data := string(buf[:n])
			if strings.HasPrefix(data, "SCREENSHOT:") {
				parts := strings.SplitN(data, ":", 6)
				if len(parts) < 5 {
					fmt.Println("Invalid packet format from", clientAddr)
					continue
				}
				clientID, hostname, ip, imgSizeStr := parts[1], parts[2], parts[3], parts[4]
				imgSize := 0
				if _, err := fmt.Sscanf(imgSizeStr, "%d", &imgSize); err != nil {
					fmt.Println("Invalid image size from", clientID, ":", err)
					continue
				}
				headerLen := len(parts[0]) + len(parts[1]) + len(parts[2]) + len(parts[3]) + len(parts[4]) + 5
				if headerLen+imgSize > n {
					fmt.Println("Incomplete image data from", clientID, ": expected", headerLen+imgSize, "got", n)
					continue
				}
				imgData := buf[headerLen:n]
				if len(imgData) != imgSize {
					fmt.Println("Mismatched image size from", clientID, ": expected", imgSize, "got", len(imgData))
					continue
				}
				mu.Lock()
				clients[clientID] = ClientInfo{Hostname: hostname, IP: ip}
				mu.Unlock()
				saveScreenshot(clientID, hostname, ip, imgData)
				_, err = conn.WriteToUDP([]byte("ACK:"+clientID), clientAddr)
				if err != nil {
					fmt.Println("Error sending ACK to", clientAddr, ":", err)
				}
			} else {
				fmt.Println("Unknown packet from", clientAddr, ":", data)
			}
		}
	}()

	// CLI chỉ hiển thị danh sách client
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := scanner.Text()
		if input == "list" {
			mu.Lock()
			fmt.Println("Connected clients:", len(clients))
			for id, info := range clients {
				fmt.Printf("- %s (Hostname: %s, IP: %s)\n", id, info.Hostname, info.IP)
			}
			mu.Unlock()
		} else {
			fmt.Println("Invalid command. Use 'list' to show connected clients.")
		}
	}
}
