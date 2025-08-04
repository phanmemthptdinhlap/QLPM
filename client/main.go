package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"net"
	"os"
	"time"

	"github.com/kardianos/service"
	"github.com/kbinani/screenshot"
	"github.com/nfnt/resize"
)

const (
	SERVER_ADDR = "192.168.1.4:8888" // Địa chỉ server
	MAX_SIZE    = 65507              // Giới hạn kích thước gói UDP
)

var logger service.Logger

type program struct {
	conn     *net.UDPConn
	clientID string
	hostname string
	stopCh   chan struct{} // Channel để dừng dịch vụ
}

func (p *program) Start(s service.Service) error {
	log.Println("Dịch vụ đang khởi chạy...")
	var err error
	addr, err := net.ResolveUDPAddr("udp", SERVER_ADDR)
	if err != nil {
		logger.Error("Lỗi phân giải địa chỉ máy chủ:", err)
		return err
	}
	p.conn, err = net.DialUDP("udp", nil, addr)
	if err != nil {
		logger.Error("Lỗi kết nối đến máy chủ:", err)
		return err
	}

	// Khởi tạo channel để dừng
	p.stopCh = make(chan struct{})

	// Lấy hostname và IP
	p.hostname, p.clientID, err = getHostnameAndIP()
	if err != nil {
		logger.Error("Lỗi lấy thông tin hostname/IP:", err)
		return err
	}

	// Chạy logic chính trong goroutine
	go p.run()
	return nil
}

func (p *program) run() {
	// Nhận ACK từ server
	go func() {
		listener, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
		if err != nil {
			logger.Error("Lỗi khởi tạo nhận dữ liệu:", err)
			return
		}
		defer listener.Close()
		buf := make([]byte, MAX_SIZE)
		for {
			select {
			case <-p.stopCh:
				return
			default:
				n, _, err := listener.ReadFromUDP(buf)
				if err != nil {
					logger.Error("Lỗi nhận dữ liệu:", err)
					continue
				}
				data := string(buf[:n])
				if data == "ACK:"+p.clientID {
					logger.Info("Đã nhận ACK từ máy chủ")
				}
			}
		}
	}()

	// Gửi ảnh màn hình
	for {
		select {
		case <-p.stopCh:
			return
		default:
			imgData, err := captureScreenshot()
			if err != nil {
				logger.Error("Lỗi chụp hình:", err)
				time.Sleep(60 * time.Second)
				continue
			}
			header := fmt.Sprintf("Thông tin hình ảnh:%s:%s:%s:%d:", p.clientID, p.hostname, p.clientID, len(imgData))
			packet := append([]byte(header), imgData...)
			if len(packet) > MAX_SIZE {
				logger.Error("Gói tin quá lớn, không thể gửi")
				time.Sleep(60 * time.Second)
				continue
			}
			_, err = p.conn.Write(packet)
			if err != nil {
				logger.Error("Lỗi gửi hình ảnh:", err)
			} else {
				logger.Info("Đã gửi hình ảnh đến máy chủ")
			}
			time.Sleep(60 * time.Second)
		}
	}
}

func (p *program) Stop(s service.Service) error {
	log.Println("Dịch vụ đang dừng...")
	// Đóng channel để dừng các goroutine
	close(p.stopCh)
	// Đóng kết nối UDP
	if p.conn != nil {
		p.conn.Close()
	}
	return nil
}

func captureScreenshot() ([]byte, error) {
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, fmt.Errorf("Không có hình ảnh")
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
	svcConfig := &service.Config{
		Name:        "QLPM",
		DisplayName: "Quản lý phòng máy",
		Description: "Ứng dụng theo dõi, giám sát máy tính học sinh",
		Option: service.KeyValue{
			"StartType": "automatic", // Tự động khởi động
		},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}

	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}
