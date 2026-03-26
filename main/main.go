package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"scan_ip/main/internal"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Tìm thư mục gốc project (chứa config/)
	rootDir := findRootDir()

	switch command {
	case "check":
		cmdCheck()
	case "init":
		cmdInit(rootDir)
	case "import":
		cmdImport(rootDir, os.Args[2:])
	case "scan":
		cmdScan(rootDir, os.Args[2:])
	case "reset-scan":
		cmdReset(rootDir)
	case "backup":
		cmdBackup(rootDir)
	case "restore":
		cmdRestore(rootDir)
	default:
		fmt.Printf("Lệnh không hợp lệ: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

// cmdCheck kiểm tra tất cả tool cần thiết
func cmdCheck() {
	fmt.Println()
	if !internal.CheckAllTools() {
		os.Exit(1)
	}
}

// cmdInit khởi tạo database schema
func cmdInit(rootDir string) {
	cfg := loadConfig(rootDir)

	// Kiểm tra MariaDB có đang chạy không
	if err := internal.CheckMariaDBService(cfg); err != nil {
		printDBError()
		log.Fatalf("Không thể kết nối MariaDB: %v", err)
	}

	db, err := internal.InitDB(cfg)
	if err != nil {
		printDBError()
		log.Fatalf("Lỗi kết nối DB: %v", err)
	}
	defer db.Close()

	if err := internal.SetupTables(db); err != nil {
		log.Fatalf("Lỗi tạo bảng: %v", err)
	}

	fmt.Println("Khởi tạo DB thành công!")
}

// cmdImport nhập IP từ file CIDR
func cmdImport(rootDir string, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	filePath := fs.String("file", filepath.Join(rootDir, "data", "vietnam.txt"), "Đường dẫn file CIDR")
	fs.Parse(args)

	cfg := loadConfig(rootDir)
	db, err := internal.InitDB(cfg)
	if err != nil {
		printDBError()
		log.Fatalf("Lỗi kết nối DB: %v", err)
	}
	defer db.Close()

	log.Printf("Đang đọc file %s để lấy dải IP...\n", *filePath)
	if err := internal.ImportIPs(db, *filePath); err != nil {
		log.Fatalf("Lỗi import: %v", err)
	}
}

// cmdScan quét IP
func cmdScan(rootDir string, args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	scanCfg := internal.DefaultScanConfig()

	fs.IntVar(&scanCfg.Limit, "limit", scanCfg.Limit, "Số IP tối đa cần quét")
	fs.IntVar(&scanCfg.Workers, "workers", scanCfg.Workers, "Số goroutine worker")
	fs.StringVar(&scanCfg.Ports, "ports", scanCfg.Ports, "Dải cổng (vd: 22,80,443)")
	fs.IntVar(&scanCfg.Rate, "rate", scanCfg.Rate, "Tốc độ masscan (packets/sec)")
	fs.Parse(args)

	// Kiểm tra masscan trước khi quét
	fmt.Println()
	if !internal.CheckAllTools() {
		os.Exit(1)
	}

	cfg := loadConfig(rootDir)
	db, err := internal.InitDB(cfg)
	if err != nil {
		printDBError()
		log.Fatalf("Lỗi kết nối DB: %v", err)
	}
	defer db.Close()

	if err := internal.ScanHosts(db, scanCfg); err != nil {
		log.Fatalf("Lỗi quét: %v", err)
	}
}

// cmdReset xóa sạch kết quả quét
func cmdReset(rootDir string) {
	fmt.Println("\n=======================================================")
	fmt.Println("⚠️  CẢNH BÁO: Bạn đang thực hiện RESET kết quả quét!")
	fmt.Println("Các hành động sau sẽ được thực thi trên database:")
	fmt.Println("  1. Bảng 'hosts':")
	fmt.Println("     - Đặt lại 'is_alive' = 0 (false)")
	fmt.Println("     - Đặt lại 'last_scan' = NULL")
	fmt.Println("     - Đặt lại 'latency_ms' = NULL")
	fmt.Println("     (Áp dụng cho TOÀN BỘ IP trong hệ thống)")
	fmt.Println("  2. Bảng 'services':")
	fmt.Println("     - XÓA TOÀN BỘ dữ liệu cổng và dịch vụ đã phát hiện.")
	fmt.Println("\nLưu ý: Danh sách IP đã import vẫn được giữ nguyên.")
	fmt.Println("Hành động này KHÔNG THỂ hoàn tác.")
	fmt.Println("=======================================================")

	if !confirmAction("\nBạn có chắc chắn muốn xóa hết kết quả quét không? (y/n): ") {
		fmt.Println("❌ Đã hủy bỏ thao tác reset-scan.")
		return
	}

	fmt.Println("\n❗ XÁC NHẬN LẦN CUỐI:")
	if !confirmAction("Bạn CỰC KỲ chắc chắn muốn thực hiện việc này chứ? (y/n): ") {
		fmt.Println("❌ Đã hủy bỏ thao tác reset-scan.")
		return
	}

	cfg := loadConfig(rootDir)
	db, err := internal.InitDB(cfg)
	if err != nil {
		printDBError()
		log.Fatalf("Lỗi kết nối DB: %v", err)
	}
	defer db.Close()

	if err := internal.ResetScanData(db); err != nil {
		log.Fatalf("Lỗi reset dữ liệu: %v", err)
	}
}

// cmdBackup sao lưu và nén database
func cmdBackup(rootDir string) {
	cfg := loadConfig(rootDir)

	// Tạo thư mục backup_database/ nếu chưa có
	backupDir := filepath.Join(rootDir, "backup_database")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Fatalf("Không thể tạo thư mục backup: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("backup_%s_%s.sql.gz", cfg.DBName, timestamp)
	filePath := filepath.Join(backupDir, fileName)

	log.Printf("Bắt đầu sao lưu database '%s'...\n", cfg.DBName)

	// Chuẩn bị lệnh mysqldump
	args := []string{
		"-h", cfg.Host,
		"-P", cfg.Port,
		"-u", cfg.User,
	}
	if cfg.Password != "" {
		args = append(args, "-p"+cfg.Password)
	}
	args = append(args, cfg.DBName)

	cmd := exec.Command("mysqldump", args...)

	// Pipe stdout to gzip
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Lỗi tạo pipe: %v", err)
	}

	// Tạo file nén
	outFile, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("Không thể tạo file backup: %v", err)
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Chạy lệnh
	if err := cmd.Start(); err != nil {
		log.Fatalf("Lỗi khởi chạy mysqldump: %v", err)
	}

	// Copy data từ mysqldump sang gzip writer
	_, err = io.Copy(gzipWriter, stdout)
	if err != nil {
		log.Fatalf("Lỗi ghi dữ liệu: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		log.Fatalf("mysqldump kết thúc với lỗi: %v", err)
	}

	// Lấy dung lượng file sau khi nén
	fi, _ := outFile.Stat()
	log.Printf("Sao lưu thành công! File: %s (%d bytes)\n", filePath, fi.Size())
}

// cmdRestore khôi phục database từ file nén
func cmdRestore(rootDir string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	filePath := fs.String("file", "", "Đường dẫn file .sql.gz (bắt buộc)")
	fs.Parse(os.Args[2:])

	if *filePath == "" {
		fmt.Println("\n❌ Lỗi: Bạn phải chỉ định file backup bằng tham số -file <path>")
		fmt.Println("Ví dụ: go run ./main restore -file backups/backup_scan_ip_xxx.sql.gz")
		os.Exit(1)
	}

	fmt.Printf("\n=======================================================\n")
	fmt.Printf("⚠️  CẢNH BÁO: Bắt đầu KHÔI PHỤC từ: %s\n", *filePath)
	fmt.Println("Hành động này sẽ GHI ĐÈ toàn bộ dữ liệu hiện tại trong database.")
	fmt.Println("=======================================================")

	if !confirmAction("\nBạn có thực sự muốn khôi phục không? (y/n): ") {
		fmt.Println("❌ Đã hủy bỏ thao tác restore.")
		return
	}

	cfg := loadConfig(rootDir)
	log.Printf("Đang khôi phục dữ liệu vào database '%s'...\n", cfg.DBName)

	// Mở file nén
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Không thể mở file backup: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		log.Fatalf("Lỗi tạo gzip reader (có thể file không đúng định dạng): %v", err)
	}
	defer gzipReader.Close()

	// Chuẩn bị lệnh mysql
	// Thêm các flag tối ưu cho việc import lớn:
	// - --init-command: Tăng timeout để tránh Deadlock/Timeout trên bảng lớn
	// - --connect-timeout: Tăng thời gian chờ kết nối
	args := []string{
		"-h", cfg.Host,
		"-P", cfg.Port,
		"-u", cfg.User,
		"--max-allowed-packet=1073741824",
		"--init-command=SET SESSION innodb_lock_wait_timeout=86400, net_read_timeout=86400, net_write_timeout=86400;",
	}
	if cfg.Password != "" {
		args = append(args, "-p"+cfg.Password)
	}
	args = append(args, cfg.DBName)

	cmd := exec.Command("mysql", args...)
	cmd.Stdin = gzipReader

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Lỗi khôi phục database: %v\nOutput: %s", err, string(output))
	}

	log.Println("✓ Khôi phục database thành công!")
}

// confirmAction hỏi người dùng xác nhận
func confirmAction(message string) bool {
	var response string
	fmt.Print(message)
	_, err := fmt.Scanln(&response)
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// printDBError in ra cảnh báo khi nối DB thất bại
func printDBError() {
	fmt.Println("\n=======================================================")
	fmt.Println("❌ LỖI: KHÔNG THỂ KẾT NỐI TỚI CƠ SỞ DỮ LIỆU MARIADB!")
	fmt.Println("=> Vui lòng kiểm tra các mục sau:")
	fmt.Println("  1. Dịch vụ MariaDB đã được BẬT chưa?")
	fmt.Println("     - Linux/Kali: sudo systemctl start mariadb")
	fmt.Println("     - macOS: brew services start mariadb")
	fmt.Println("     - Windows: Mở Services (services.msc) và start MariaDB")
	fmt.Println("  2. Cấu hình trong file 'config/database.txt' đã đúng chưa?")
	fmt.Println("=======================================================")
}

// loadConfig đọc config từ file
func loadConfig(rootDir string) *internal.DBConfig {
	configPath := filepath.Join(rootDir, "config", "database.txt")
	cfg, err := internal.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Lỗi đọc config: %v", err)
	}
	return cfg
}

// findRootDir tìm thư mục gốc project
func findRootDir() string {
	// Ưu tiên dùng working directory (hoạt động đúng với cả go run và binary)
	wd, err := os.Getwd()
	if err == nil {
		// Nếu đang ở main/ thì lùi lại 1 cấp
		if filepath.Base(wd) == "main" {
			wd = filepath.Dir(wd)
		}
		// Kiểm tra config/ có tồn tại
		if _, err := os.Stat(filepath.Join(wd, "config")); err == nil {
			return wd
		}
	}

	// Fallback: dùng thư mục của executable (khi chạy binary đã build)
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if filepath.Base(dir) == "main" {
			dir = filepath.Dir(dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "config")); err == nil {
			return dir
		}
	}

	return wd
}

func printUsage() {
	fmt.Println(`Scan IP Toolkit - Quét IP tự động

Cách dùng:
  scan_ip <lệnh> [tùy chọn]

Các lệnh:
  check                    Kiểm tra các công cụ cần thiết (masscan, mariadb)
  init                     Khởi tạo database schema
  import [tùy chọn]        Nhập dải IP từ file CIDR
  scan   [tùy chọn]        Quét IP với masscan
  reset-scan               Xóa sạch kết quả quét (đưa DB về trạng thái chưa quét)
  backup                   Sao lưu toàn bộ database và nén (gzip)
  restore -file <path>     Khôi phục database từ file nén (.sql.gz)

Tùy chọn lệnh import:
  -file <path>             Đường dẫn file CIDR (mặc định: data/vietnam.txt)

Tùy chọn lệnh scan:
  -limit  <n>              Số IP tối đa (mặc định: 100000, 0 = không giới hạn)
  -workers <n>             Số goroutine (mặc định: 10)
  -ports  <ports>          Dải cổng (mặc định: --ping, chỉ kiểm tra sống/chết)
  -rate   <n>              Tốc độ masscan (mặc định: 1000)

Ví dụ:
  scan_ip check
  scan_ip init
  scan_ip import -file data/vietnam.txt
  scan_ip scan -limit 5000 -workers 20 -rate 2000
  scan_ip scan -ports 80,443 -rate 5000`)
}
