package internal

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

// InitDB kết nối tới MariaDB và trả về *sql.DB
func InitDB(cfg *DBConfig) (*sql.DB, error) {
	// Thử kết nối không có DB trước để tạo DB nếu chưa có
	dbNoDB, err := sql.Open("mysql", cfg.DSNWithoutDB())
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối MariaDB: %w", err)
	}

	// Tạo database nếu chưa tồn tại
	_, err = dbNoDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci", cfg.DBName))
	if err != nil {
		dbNoDB.Close()
		return nil, fmt.Errorf("không thể tạo database '%s': %w", cfg.DBName, err)
	}
	dbNoDB.Close()

	// Kết nối tới database đã tạo
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối database '%s': %w", cfg.DBName, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("MariaDB không phản hồi: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)

	return db, nil
}

// SetupTables tạo các bảng cần thiết trong MariaDB
func SetupTables(db *sql.DB) error {
	tables := []string{
		// Bảng lưu dải IP của ISP
		`CREATE TABLE IF NOT EXISTS isp_ranges (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			cidr        VARCHAR(18)  NOT NULL COMMENT 'Dải CIDR, ví dụ: 1.2.3.0/24',
			isp_name    VARCHAR(255) DEFAULT '' COMMENT 'Tên nhà mạng',
			country     VARCHAR(10)  DEFAULT 'VN',
			created_at  DATETIME     DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_cidr (cidr)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		// Bảng lưu từng IP đơn
		`CREATE TABLE IF NOT EXISTS hosts (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			ip_address  VARCHAR(15)  NOT NULL COMMENT 'Địa chỉ IPv4',
			range_id    INT          DEFAULT NULL COMMENT 'FK tới isp_ranges',
			is_alive    TINYINT(1)   DEFAULT 0 COMMENT '1=sống, 0=chưa biết/chết',
			last_scan   DATETIME     DEFAULT NULL,
			latency_ms  FLOAT        DEFAULT NULL COMMENT 'Thời gian phản hồi (ms)',
			created_at  DATETIME     DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_ip (ip_address),
			KEY idx_alive (is_alive),
			KEY idx_last_scan (last_scan),
			KEY fk_range (range_id),
			CONSTRAINT fk_hosts_range FOREIGN KEY (range_id) REFERENCES isp_ranges(id) ON DELETE SET NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		// Bảng lưu dịch vụ/cổng mở trên mỗi IP
		`CREATE TABLE IF NOT EXISTS services (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			host_id     INT          NOT NULL COMMENT 'FK tới hosts',
			port        INT          NOT NULL,
			protocol    VARCHAR(10)  DEFAULT 'tcp' COMMENT 'tcp hoặc udp',
			service     VARCHAR(100) DEFAULT '' COMMENT 'Tên dịch vụ (http, ssh...)',
			banner      TEXT         DEFAULT NULL COMMENT 'Banner grab',
			state       VARCHAR(20)  DEFAULT 'open' COMMENT 'open, closed, filtered',
			found_at    DATETIME     DEFAULT CURRENT_TIMESTAMP,
			KEY idx_host (host_id),
			KEY idx_port (port),
			CONSTRAINT fk_services_host FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("lỗi tạo bảng: %w", err)
		}
	}

	log.Println("Đã khởi tạo Database schema thành công.")
	return nil
}

// ResetScanData xóa sạch kết quả quét (is_alive = 0, last_scan = NULL) và xóa bảng services
func ResetScanData(db *sql.DB) error {
	log.Println("Đang reset dữ liệu quét...")

	// 1. Reset bảng hosts
	_, err := db.Exec("UPDATE hosts SET is_alive = 0, last_scan = NULL, latency_ms = NULL")
	if err != nil {
		return fmt.Errorf("lỗi reset bảng hosts: %w", err)
	}

	// 2. Xóa bảng services
	// Truncate có thể lỗi nếu có ràng buộc, dùng DELETE cho chắc chắn và an toàn
	_, err = db.Exec("DELETE FROM services")
	if err != nil {
		return fmt.Errorf("lỗi xóa bảng services: %w", err)
	}

	log.Println("Đã xóa sạch tác động của scan. Dữ liệu đã trở về trạng thái ban đầu.")
	return nil
}
