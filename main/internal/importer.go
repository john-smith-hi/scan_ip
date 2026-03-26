package internal

import (
	"bufio"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

const batchSize = 10000

// ImportIPs đọc file CIDR và chèn từng IP vào bảng hosts
func ImportIPs(db *sql.DB, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("không thể mở file '%s': %w", filePath, err)
	}
	defer file.Close()

	// Đọc tất cả dải CIDR từ file
	var cidrs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cidrs = append(cidrs, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("lỗi đọc file: %w", err)
	}

	log.Printf("Đã lấy được %d dải CIDR. Bắt đầu chèn vào CSDL...\n", len(cidrs))

	totalInserted := 0

	for _, cidr := range cidrs {
		// Chèn dải CIDR vào bảng isp_ranges
		rangeID, err := insertRange(db, cidr)
		if err != nil {
			log.Printf("Cảnh báo: không thể chèn dải %s — %v", cidr, err)
			continue
		}

		// Parse CIDR thành danh sách IP
		ips, err := expandCIDR(cidr)
		if err != nil {
			log.Printf("Cảnh báo: CIDR không hợp lệ '%s' — %v", cidr, err)
			continue
		}

		// Batch insert
		for i := 0; i < len(ips); i += batchSize {
			end := i + batchSize
			if end > len(ips) {
				end = len(ips)
			}
			batch := ips[i:end]

			if err := batchInsertHosts(db, batch, rangeID); err != nil {
				log.Printf("Lỗi chèn batch: %v", err)
				continue
			}
			totalInserted += len(batch)
		}
	}

	log.Printf("Hoàn tất chèn dữ liệu! Tổng số IP đã chèn: %d\n", totalInserted)
	return nil
}

// insertRange chèn dải CIDR vào isp_ranges, trả về id
func insertRange(db *sql.DB, cidr string) (int64, error) {
	result, err := db.Exec(
		"INSERT IGNORE INTO isp_ranges (cidr) VALUES (?)", cidr,
	)
	if err != nil {
		return 0, err
	}

	id, _ := result.LastInsertId()
	if id == 0 {
		// Dải đã tồn tại, lấy id
		row := db.QueryRow("SELECT id FROM isp_ranges WHERE cidr = ?", cidr)
		if err := row.Scan(&id); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// batchInsertHosts chèn nhiều IP cùng lúc
func batchInsertHosts(db *sql.DB, ips []string, rangeID int64) error {
	if len(ips) == 0 {
		return nil
	}

	query := "INSERT IGNORE INTO hosts (ip_address, range_id) VALUES "
	vals := make([]interface{}, 0, len(ips)*2)
	placeholders := make([]string, 0, len(ips))

	for _, ip := range ips {
		placeholders = append(placeholders, "(?, ?)")
		vals = append(vals, ip, rangeID)
	}

	query += strings.Join(placeholders, ",")
	_, err := db.Exec(query, vals...)
	return err
}

// expandCIDR chuyển CIDR thành danh sách IP đơn
func expandCIDR(cidr string) ([]string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}

	// Bỏ network address và broadcast address nếu có nhiều hơn 2 IP
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}

	return ips, nil
}

// incrementIP tăng IP lên 1
func incrementIP(ip net.IP) {
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	val := binary.BigEndian.Uint32(ip4)
	val++
	binary.BigEndian.PutUint32(ip4, val)
}
