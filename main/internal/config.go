package internal

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// DBConfig chứa thông tin kết nối MariaDB
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// DSN trả về chuỗi kết nối cho go-sql-driver/mysql
func (c *DBConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4",
		c.User, c.Password, c.Host, c.Port, c.DBName)
}

// DSNWithoutDB trả về DSN không có tên database (dùng để tạo DB)
func (c *DBConfig) DSNWithoutDB() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true&charset=utf8mb4",
		c.User, c.Password, c.Host, c.Port)
}

// LoadConfig đọc file config/database.txt và trả về DBConfig
func LoadConfig(path string) (*DBConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("không thể đọc file config: %w", err)
	}
	defer file.Close()

	cfg := &DBConfig{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "host":
			cfg.Host = value
		case "port":
			cfg.Port = value
		case "user":
			cfg.User = value
		case "password":
			cfg.Password = value
		case "dbname":
			cfg.DBName = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lỗi đọc file config: %w", err)
	}

	// Giá trị mặc định
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == "" {
		cfg.Port = "3306"
	}
	if cfg.User == "" {
		cfg.User = "root"
	}
	if cfg.DBName == "" {
		cfg.DBName = "scan_ip"
	}

	return cfg, nil
}
