package internal

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
)

// ScanConfig cấu hình cho scanner
type ScanConfig struct {
	Limit   int    // Số IP tối đa cần quét
	Workers int    // Số goroutine worker
	Ports   string // Dải cổng quét (ví dụ: "22,80,443,3306,8080")
	Rate    int    // Tốc độ gửi gói (packets/sec) cho masscan
}

// DefaultScanConfig trả về cấu hình mặc định
func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		Limit:   100000,
		Workers: 10,
		Ports:   "", // Mặc định là chuỗi rỗng -> sẽ dùng --ping (kiểm tra sống/chết)
		Rate:    1000,
	}
}

// ScanHosts lấy IP chưa quét từ DB, chạy masscan, và cập nhật kết quả
func ScanHosts(db *sql.DB, cfg ScanConfig) error {
	// Lấy danh sách IP chưa quét hoặc quét cũ
	ips, err := getUnscannedIPs(db, cfg.Limit)
	if err != nil {
		return fmt.Errorf("lỗi lấy danh sách IP: %w", err)
	}

	if len(ips) == 0 {
		log.Println("Không có IP nào cần quét.")
		return nil
	}

	log.Printf("Đã lấy %d IP để quét. Bắt đầu quét với %d workers...\n", len(ips), cfg.Workers)

	// Chia IP thành các batch nhỏ cho mỗi worker
	batchSize := len(ips) / cfg.Workers
	if batchSize < 1 {
		batchSize = 1
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, cfg.Workers)

	for i := 0; i < len(ips); i += batchSize {
		end := i + batchSize
		if end > len(ips) {
			end = len(ips)
		}
		batch := ips[i:end]

		wg.Add(1)
		sem <- struct{}{} // Giới hạn goroutine

		go func(ipBatch []string) {
			defer wg.Done()
			defer func() { <-sem }()

			processBatch(db, ipBatch, cfg)
		}(batch)
	}

	wg.Wait()
	log.Println("Quét hoàn tất!")
	return nil
}

// getUnscannedIPs lấy danh sách IP chưa quét từ DB
func getUnscannedIPs(db *sql.DB, limit int) ([]string, error) {
	query := `
		SELECT ip_address FROM hosts 
		WHERE last_scan IS NULL 
		ORDER BY id ASC
		LIMIT ?
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}

	return ips, rows.Err()
}

// processBatch xử lý một batch IP bằng masscan
func processBatch(db *sql.DB, ips []string, cfg ScanConfig) {
	results, err := RunMasscan(ips, cfg.Ports, cfg.Rate)
	if err != nil {
		log.Printf("Lỗi masscan batch: %v", err)
		// Đánh dấu đã quét (nhưng không sống) cho tất cả IP trong batch
		batchUpdateHosts(db, ips, false)
		return
	}

	// Tạo map IP có phản hồi
	aliveIPs := make(map[string]bool)
	for _, r := range results {
		aliveIPs[r.IP] = true

		// Lưu dịch vụ vào DB
		for _, port := range r.Ports {
			if err := insertService(db, r.IP, port.Port, port.Protocol, port.Status); err != nil {
				log.Printf("Lỗi lưu service %s:%d — %v", r.IP, port.Port, err)
			}
		}
	}

	// Phân loại IP sống/chết để batch update
	var aliveList []string
	var deadList []string
	for _, ip := range ips {
		if aliveIPs[ip] {
			aliveList = append(aliveList, ip)
		} else {
			deadList = append(deadList, ip)
		}
	}

	// Batch update trạng thái
	if len(aliveList) > 0 {
		batchUpdateHosts(db, aliveList, true)
	}
	if len(deadList) > 0 {
		batchUpdateHosts(db, deadList, false)
	}

	log.Printf("  Batch hoàn tất: %d/%d IP sống", len(aliveIPs), len(ips))
}

// batchUpdateHosts cập nhật hàng loạt trạng thái IP
func batchUpdateHosts(db *sql.DB, ips []string, alive bool) {
	if len(ips) == 0 {
		return
	}

	isAlive := 0
	if alive {
		isAlive = 1
	}

	// MariaDB giới hạn placeholder (65535).
	// Chúng ta dùng 10k IP mỗi batch query cho an toàn.
	const limit = 10000
	for i := 0; i < len(ips); i += limit {
		end := i + limit
		if end > len(ips) {
			end = len(ips)
		}
		subBatch := ips[i:end]

		placeholders := make([]string, len(subBatch))
		args := make([]interface{}, len(subBatch)+1)
		args[0] = isAlive
		for j, ip := range subBatch {
			placeholders[j] = "?"
			args[j+1] = ip
		}

		query := fmt.Sprintf(
			"UPDATE hosts SET is_alive = ?, last_scan = NOW() WHERE ip_address IN (%s)",
			strings.Join(placeholders, ","),
		)

		_, err := db.Exec(query, args...)
		if err != nil {
			log.Printf("Lỗi batch update hosts: %v", err)
		}
	}
}

// insertService chèn dịch vụ phát hiện được
func insertService(db *sql.DB, ip string, port int, protocol string, state string) error {
	_, err := db.Exec(`
		INSERT INTO services (host_id, port, protocol, state)
		SELECT id, ?, ?, ?
		FROM hosts WHERE ip_address = ?
		ON DUPLICATE KEY UPDATE state = VALUES(state), found_at = CURRENT_TIMESTAMP
	`, port, protocol, state, ip)
	return err
}
